// Teleport
// Copyright (C) 2025 Gravitational, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package vnet

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"sync"

	"github.com/gravitational/trace"

	"github.com/gravitational/teleport/api"
	vnetv1 "github.com/gravitational/teleport/gen/proto/go/teleport/lib/vnet/v1"
)

// clientApplicationService implements the gRPC
// [vnetv1.ClientApplicationServiceServer] to expose functionality that requires
// a Teleport client to the VNet admin service running in another process.
type clientApplicationService struct {
	// opt-in to compilation errors if this doesn't implement
	// [vnetv1.ClientApplicationServiceServer]
	vnetv1.UnsafeClientApplicationServiceServer

	cfg *clientApplicationServiceConfig

	// networkStackInfo will receive any network stack info reported via
	// ReportNetworkStackInfo.
	networkStackInfo chan *vnetv1.NetworkStackInfo

	// mu protects appSignerCache
	mu sync.Mutex
	// appSignerCache caches the crypto.Signer for each certificate issued by
	// ReissueAppCert so that SignForApp can later use that signer.
	//
	// Signers are never deleted from the map. When the cert expires, the local
	// proxy in the admin process will detect the cert expiry and call
	// ReissueAppCert, which will overwrite the signer for the app with a new
	// one.
	appSignerCache map[appKey]crypto.Signer
}

type clientApplicationServiceConfig struct {
	fqdnResolver          *fqdnResolver
	localOSConfigProvider *LocalOSConfigProvider
	clientApplication     ClientApplication
}

func newClientApplicationService(cfg *clientApplicationServiceConfig) *clientApplicationService {
	return &clientApplicationService{
		cfg:              cfg,
		networkStackInfo: make(chan *vnetv1.NetworkStackInfo, 1),
		appSignerCache:   make(map[appKey]crypto.Signer),
	}
}

// AuthenticateProcess implements [vnetv1.ClientApplicationServiceServer.AuthenticateProcess].
func (s *clientApplicationService) AuthenticateProcess(ctx context.Context, req *vnetv1.AuthenticateProcessRequest) (*vnetv1.AuthenticateProcessResponse, error) {
	log.DebugContext(ctx, "Received AuthenticateProcess request from admin process")
	if req.Version != api.Version {
		return nil, trace.BadParameter("version mismatch, user process version is %s, admin process version is %s",
			api.Version, req.Version)
	}
	if err := platformAuthenticateProcess(ctx, req); err != nil {
		log.ErrorContext(ctx, "Failed to authenticate process", "error", err)
		return nil, trace.Wrap(err, "authenticating process")
	}
	return &vnetv1.AuthenticateProcessResponse{
		Version: api.Version,
	}, nil
}

// ReportNetworkStackInfo implements [vnetv1.ClientApplicationServiceServer.ReportNetworkStackInfo].
func (s *clientApplicationService) ReportNetworkStackInfo(ctx context.Context, req *vnetv1.ReportNetworkStackInfoRequest) (*vnetv1.ReportNetworkStackInfoResponse, error) {
	select {
	case s.networkStackInfo <- req.GetNetworkStackInfo():
	default:
		return nil, trace.BadParameter("ReportNetworkStackInfo must be called exactly once")
	}
	return &vnetv1.ReportNetworkStackInfoResponse{}, nil
}

// Ping implements [vnetv1.ClientApplicationServiceServer.Ping].
func (s *clientApplicationService) Ping(ctx context.Context, req *vnetv1.PingRequest) (*vnetv1.PingResponse, error) {
	return &vnetv1.PingResponse{}, nil
}

// ResolveFQDN implements [vnetv1.ClientApplicationServiceServer.ResolveFQDN].
func (s *clientApplicationService) ResolveFQDN(ctx context.Context, req *vnetv1.ResolveFQDNRequest) (*vnetv1.ResolveFQDNResponse, error) {
	resp, err := s.cfg.fqdnResolver.ResolveFQDN(ctx, req.GetFqdn())
	return resp, trace.Wrap(err, "resolving FQDN")
}

// ReissueAppCert implements [vnetv1.ClientApplicationServiceServer.ReissueAppCert].
// It caches the signer issued for each app so that it can later be used to
// issue signatures in [clientApplicationService.SignForApp].
func (s *clientApplicationService) ReissueAppCert(ctx context.Context, req *vnetv1.ReissueAppCertRequest) (*vnetv1.ReissueAppCertResponse, error) {
	appInfo := req.GetAppInfo()
	if appInfo == nil {
		return nil, trace.BadParameter("missing AppInfo")
	}
	appKey := appInfo.GetAppKey()
	if err := checkAppKey(appKey); err != nil {
		return nil, trace.Wrap(err)
	}
	cert, err := s.cfg.clientApplication.ReissueAppCert(ctx, appInfo, uint16(req.GetTargetPort()))
	if err != nil {
		return nil, trace.Wrap(err, "reissuing app certificate")
	}
	s.setSignerForApp(appKey, uint16(req.GetTargetPort()), cert.PrivateKey.(crypto.Signer))
	return &vnetv1.ReissueAppCertResponse{
		Cert: cert.Certificate[0],
	}, nil
}

// SignForApp implements [vnetv1.ClientApplicationServiceServer.SignForApp].
// It uses a cached signer for the requested app, which must have previously
// been issued a certificate via [clientApplicationService.ReissueAppCert].
func (s *clientApplicationService) SignForApp(ctx context.Context, req *vnetv1.SignForAppRequest) (*vnetv1.SignForAppResponse, error) {
	signReq := req.GetSign()
	log.DebugContext(ctx, "Got SignForApp request",
		"app", req.GetAppKey(),
		"hash", signReq.GetHash(),
		"is_rsa_pss", signReq.PssSaltLength != nil,
		"pss_salt_len", signReq.GetPssSaltLength(),
		"digest_len", len(signReq.GetDigest()),
	)

	appKey := req.GetAppKey()
	if err := checkAppKey(appKey); err != nil {
		return nil, trace.Wrap(err)
	}
	signer, ok := s.getSignerForApp(req.GetAppKey(), uint16(req.GetTargetPort()))
	if !ok {
		return nil, trace.BadParameter("no signer for app %v", appKey)
	}

	signature, err := sign(signer, signReq)
	if err != nil {
		return nil, trace.Wrap(err, "signing for app %v", appKey)
	}
	return &vnetv1.SignForAppResponse{
		Signature: signature,
	}, nil
}

func sign(signer crypto.Signer, signReq *vnetv1.SignRequest) ([]byte, error) {
	var hash crypto.Hash
	switch signReq.GetHash() {
	case vnetv1.Hash_HASH_NONE:
		hash = crypto.Hash(0)
	case vnetv1.Hash_HASH_SHA256:
		hash = crypto.SHA256
	default:
		return nil, trace.BadParameter("unsupported hash %v", signReq.GetHash())
	}
	opts := crypto.SignerOpts(hash)
	if signReq.PssSaltLength != nil {
		opts = &rsa.PSSOptions{
			Hash:       hash,
			SaltLength: int(*signReq.PssSaltLength),
		}
	}
	signature, err := signer.Sign(rand.Reader, signReq.GetDigest(), opts)
	return signature, trace.Wrap(err)
}

func (s *clientApplicationService) setSignerForApp(appKey *vnetv1.AppKey, targetPort uint16, signer crypto.Signer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appSignerCache[newAppKey(appKey, targetPort)] = signer
}

func (s *clientApplicationService) getSignerForApp(appKey *vnetv1.AppKey, targetPort uint16) (crypto.Signer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	signer, ok := s.appSignerCache[newAppKey(appKey, targetPort)]
	return signer, ok
}

// OnNewConnection gets called whenever a new connection is about to be
// established through VNet for observability.
func (s *clientApplicationService) OnNewConnection(ctx context.Context, req *vnetv1.OnNewConnectionRequest) (*vnetv1.OnNewConnectionResponse, error) {
	if err := s.cfg.clientApplication.OnNewConnection(ctx, req.GetAppKey()); err != nil {
		return nil, trace.Wrap(err)
	}
	return &vnetv1.OnNewConnectionResponse{}, nil
}

// OnInvalidLocalPort gets called before VNet refuses to handle a connection
// to a multi-port TCP app because the provided port does not match any of the
// TCP ports in the app spec.
func (s *clientApplicationService) OnInvalidLocalPort(ctx context.Context, req *vnetv1.OnInvalidLocalPortRequest) (*vnetv1.OnInvalidLocalPortResponse, error) {
	s.cfg.clientApplication.OnInvalidLocalPort(ctx, req.GetAppInfo(), uint16(req.GetTargetPort()))
	return &vnetv1.OnInvalidLocalPortResponse{}, nil
}

// appKey is a clone of [vnetv1.AppKey] that is not a protobuf type so it can be
// used as a key in maps.
type appKey struct {
	profile, leafCluster, app string
	port                      uint16
}

func newAppKey(protoAppKey *vnetv1.AppKey, port uint16) appKey {
	return appKey{
		profile:     protoAppKey.GetProfile(),
		leafCluster: protoAppKey.GetLeafCluster(),
		app:         protoAppKey.GetName(),
		port:        port,
	}
}

// GetTargetOSConfiguration returns the configuration values that should be
// configured in the OS, including DNS zones that should be handled by the VNet
// DNS nameserver and the IPv4 CIDR ranges that should be routed to the VNet TUN
// interface.
func (s *clientApplicationService) GetTargetOSConfiguration(ctx context.Context, _ *vnetv1.GetTargetOSConfigurationRequest) (*vnetv1.GetTargetOSConfigurationResponse, error) {
	targetConfig, err := s.cfg.localOSConfigProvider.GetTargetOSConfiguration(ctx)
	if err != nil {
		return nil, trace.Wrap(err, "getting target OS configuration")
	}
	return &vnetv1.GetTargetOSConfigurationResponse{
		TargetOsConfiguration: targetConfig,
	}, nil
}

// UserTLSCert returns the user TLS certificate for a specific profile.
func (s *clientApplicationService) UserTLSCert(ctx context.Context, req *vnetv1.UserTLSCertRequest) (*vnetv1.UserTLSCertResponse, error) {
	tlsCert, err := s.cfg.clientApplication.UserTLSCert(ctx, req.GetProfile())
	if err != nil {
		return nil, trace.Wrap(err, "getting user TLS cert")
	}
	if len(tlsCert.Certificate) == 0 {
		return nil, trace.Errorf("user TLS cert has no certificate")
	}
	dialOpts, err := s.cfg.clientApplication.GetDialOptions(ctx, req.GetProfile())
	if err != nil {
		return nil, trace.Wrap(err, "getting TLS dial options")
	}
	return &vnetv1.UserTLSCertResponse{
		Cert:        tlsCert.Certificate[0],
		DialOptions: dialOpts,
	}, nil
}

// SignForUserTLS signs a digest with the user TLS private key.
func (s *clientApplicationService) SignForUserTLS(ctx context.Context, req *vnetv1.SignForUserTLSRequest) (*vnetv1.SignForUserTLSResponse, error) {
	tlsCert, err := s.cfg.clientApplication.UserTLSCert(ctx, req.GetProfile())
	if err != nil {
		return nil, trace.Wrap(err, "getting user TLS config")
	}
	signer, ok := tlsCert.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, trace.Errorf("user TLS private key does not implement crypto.Signer")
	}
	signature, err := sign(signer, req.GetSign())
	if err != nil {
		return nil, trace.Wrap(err, "signing for user TLS certificate")
	}
	return &vnetv1.SignForUserTLSResponse{
		Signature: signature,
	}, nil
}

// checkAppKey checks that at least the app profile and name are set, which are
// necessary to to disambiguate apps. LeafCluster is expected to be empty if the
// app is in a root cluster.
func checkAppKey(key *vnetv1.AppKey) error {
	switch {
	case key == nil:
		return trace.BadParameter("app key must not be nil")
	case key.GetProfile() == "":
		return trace.BadParameter("app key profile must be set")
	case key.GetName() == "":
		return trace.BadParameter("app key name must be set")
	}
	return nil
}
