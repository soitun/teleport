/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package discovery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/container/apiv1/containerpb"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/redis/armredis/v3"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/memorydb"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	rss "github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/defaults"
	discoveryconfigv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/discoveryconfig/v1"
	integrationpb "github.com/gravitational/teleport/api/gen/proto/go/teleport/integration/v1"
	usertasksv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/usertasks/v1"
	usageeventsv1 "github.com/gravitational/teleport/api/gen/proto/go/usageevents/v1"
	"github.com/gravitational/teleport/api/internalutils/stream"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/discoveryconfig"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/teleport/api/types/header"
	"github.com/gravitational/teleport/api/utils"
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/auth/authclient"
	"github.com/gravitational/teleport/lib/auth/authtest"
	"github.com/gravitational/teleport/lib/authz"
	"github.com/gravitational/teleport/lib/cloud"
	"github.com/gravitational/teleport/lib/cloud/awsconfig"
	"github.com/gravitational/teleport/lib/cloud/azure"
	"github.com/gravitational/teleport/lib/cloud/gcp"
	gcpimds "github.com/gravitational/teleport/lib/cloud/imds/gcp"
	"github.com/gravitational/teleport/lib/cloud/mocks"
	libevents "github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/modules"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/srv/discovery/common"
	"github.com/gravitational/teleport/lib/srv/discovery/fetchers"
	"github.com/gravitational/teleport/lib/srv/discovery/fetchers/db"
	"github.com/gravitational/teleport/lib/srv/server"
	usagereporter "github.com/gravitational/teleport/lib/usagereporter/teleport"
	"github.com/gravitational/teleport/lib/utils/log/logtest"
)

func TestMain(m *testing.M) {
	modules.SetInsecureTestMode(true)
	os.Exit(m.Run())
}

type mockSSMClient struct {
	server.SSMClient
	commandOutput *ssm.SendCommandOutput
	invokeOutput  *ssm.GetCommandInvocationOutput
}

func (sm *mockSSMClient) SendCommand(_ context.Context, input *ssm.SendCommandInput, _ ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {
	return sm.commandOutput, nil
}

func (sm *mockSSMClient) GetCommandInvocation(_ context.Context, input *ssm.GetCommandInvocationInput, _ ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {
	return sm.invokeOutput, nil
}

type mockEmitter struct {
	eventHandler func(*testing.T, events.AuditEvent, *Server)
	server       *Server
	t            *testing.T
}

func (me *mockEmitter) EmitAuditEvent(ctx context.Context, event events.AuditEvent) error {
	if me.eventHandler != nil {
		me.eventHandler(me.t, event, me.server)
	}
	return nil
}

type mockUsageReporter struct {
	mu                       sync.Mutex
	resourceAddedEventCount  int
	discoveryFetchEventCount int
}

func (m *mockUsageReporter) AnonymizeAndSubmit(events ...usagereporter.Anonymizable) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range events {
		switch e.(type) {
		case *usagereporter.ResourceCreateEvent:
			m.resourceAddedEventCount++
		case *usagereporter.DiscoveryFetchEvent:
			m.discoveryFetchEventCount++
		}
	}
}

func (m *mockUsageReporter) ResourceCreateEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resourceAddedEventCount
}

func (m *mockUsageReporter) DiscoveryFetchEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.discoveryFetchEventCount
}

type mockEC2Client struct {
	output *ec2.DescribeInstancesOutput
}

func (m *mockEC2Client) DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return m.output, nil
}

func genEC2InstanceIDs(n int) []string {
	var ec2InstanceIDs []string
	for i := range n {
		ec2InstanceIDs = append(ec2InstanceIDs, fmt.Sprintf("instance-id-%d", i))
	}
	return ec2InstanceIDs
}

func genEC2Instances(n int) []ec2types.Instance {
	var ec2Instances []ec2types.Instance
	for _, id := range genEC2InstanceIDs(n) {
		ec2Instances = append(ec2Instances, ec2types.Instance{
			InstanceId: aws.String(id),
			Tags: []ec2types.Tag{{
				Key:   aws.String("env"),
				Value: aws.String("dev"),
			}},
			State: &ec2types.InstanceState{
				Name: ec2types.InstanceStateNameRunning,
			},
		})
	}
	return ec2Instances
}

type mockSSMInstaller struct {
	mu                 sync.Mutex
	installedInstances map[string]struct{}
	runError           error
}

func (m *mockSSMInstaller) Run(_ context.Context, req server.SSMRunRequest) error {
	if m.runError != nil {
		return m.runError
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range req.Instances {
		m.installedInstances[inst.InstanceID] = struct{}{}
	}
	return nil
}

func (m *mockSSMInstaller) GetInstalledInstances() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.installedInstances))
	for k := range m.installedInstances {
		keys = append(keys, k)
	}
	return keys
}

func TestDiscoveryServer(t *testing.T) {
	t.Parallel()

	fakeClock := clockwork.NewFakeClock()

	defaultDiscoveryGroup := "dc001"
	defaultStaticMatcher := Matchers{
		AWS: []types.AWSMatcher{{
			Types:   []string{"ec2"},
			Regions: []string{"eu-central-1"},
			Tags:    map[string]utils.Strings{"teleport": {"yes"}},
			SSM:     &types.AWSSSM{DocumentName: "document"},
			Params: &types.InstallerParams{
				InstallTeleport: true,
			},
		}},
	}

	defaultDiscoveryConfig, err := discoveryconfig.NewDiscoveryConfig(
		header.Metadata{Name: uuid.NewString()},
		discoveryconfig.Spec{
			DiscoveryGroup: defaultDiscoveryGroup,
			AWS:            defaultStaticMatcher.AWS,
			Azure:          defaultStaticMatcher.Azure,
			GCP:            defaultStaticMatcher.GCP,
			Kube:           defaultStaticMatcher.Kubernetes,
		},
	)
	require.NoError(t, err)

	dcForEC2SSMWithIntegrationName := uuid.NewString()
	dcForEC2SSMWithIntegration, err := discoveryconfig.NewDiscoveryConfig(
		header.Metadata{Name: dcForEC2SSMWithIntegrationName},
		discoveryconfig.Spec{
			DiscoveryGroup: defaultDiscoveryGroup,
			AWS: []types.AWSMatcher{{
				Types:   []string{"ec2"},
				Regions: []string{"eu-central-1"},
				Tags:    map[string]utils.Strings{"teleport": {"yes"}},
				SSM:     &types.AWSSSM{DocumentName: "document"},
				Params: &types.InstallerParams{
					InstallTeleport: true,
					EnrollMode:      types.InstallParamEnrollMode_INSTALL_PARAM_ENROLL_MODE_SCRIPT,
				},
				Integration: "my-integration",
			}},
		},
	)
	require.NoError(t, err)

	discoveryConfigForUserTaskEC2TestName := uuid.NewString()
	discoveryConfigForUserTaskEC2Test, err := discoveryconfig.NewDiscoveryConfig(
		header.Metadata{Name: discoveryConfigForUserTaskEC2TestName},
		discoveryconfig.Spec{
			DiscoveryGroup: defaultDiscoveryGroup,
			AWS: []types.AWSMatcher{{
				Types:   []string{"ec2"},
				Regions: []string{"eu-west-2"},
				Tags:    map[string]utils.Strings{"RunDiscover": {"please"}},
				SSM:     &types.AWSSSM{DocumentName: "document"},
				Params: &types.InstallerParams{
					InstallTeleport: true,
					EnrollMode:      types.InstallParamEnrollMode_INSTALL_PARAM_ENROLL_MODE_SCRIPT,
				},
				Integration: "my-integration",
			}},
		},
	)
	require.NoError(t, err)

	dcForEC2StatusWithoutMatchName := uuid.NewString()
	dcForEC2StatusWithoutMatch, err := discoveryconfig.NewDiscoveryConfig(
		header.Metadata{Name: dcForEC2StatusWithoutMatchName},
		discoveryconfig.Spec{
			DiscoveryGroup: defaultDiscoveryGroup,
			AWS: []types.AWSMatcher{{
				Types:   []string{"ec2"},
				Regions: []string{"eu-central-1"},
				Tags:    map[string]utils.Strings{"teleport": {"yes"}},
				SSM:     &types.AWSSSM{DocumentName: "document"},
				Params: &types.InstallerParams{
					InstallTeleport: true,
					EnrollMode:      types.InstallParamEnrollMode_INSTALL_PARAM_ENROLL_MODE_SCRIPT,
				},
				Integration: "my-integration",
			}},
		},
	)
	require.NoError(t, err)

	tcs := []struct {
		name string
		// presentInstances is a list of servers already present in teleport.
		presentInstances          []types.Server
		foundEC2Instances         []ec2types.Instance
		ssm                       *mockSSMClient
		emitter                   *mockEmitter
		discoveryConfig           *discoveryconfig.DiscoveryConfig
		staticMatchers            Matchers
		wantInstalledInstances    []string
		wantDiscoveryConfigStatus *discoveryconfig.Status
		userTasksDiscoverCheck    func(t *testing.T, userTasksClt services.UserTasks)
		ssmRunError               error
	}{
		{
			name:             "no nodes present, 1 found",
			presentInstances: []types.Server{},
			foundEC2Instances: []ec2types.Instance{
				{
					InstanceId: aws.String("instance-id-1"),
					Tags: []ec2types.Tag{{
						Key:   aws.String("env"),
						Value: aws.String("dev"),
					}},
					State: &ec2types.InstanceState{
						Name: ec2types.InstanceStateNameRunning,
					},
				},
			},
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			emitter: &mockEmitter{
				eventHandler: func(t *testing.T, ae events.AuditEvent, server *Server) {
					t.Helper()
					require.Equal(t, &events.SSMRun{
						Metadata: events.Metadata{
							Type: libevents.SSMRunEvent,
							Code: libevents.SSMRunSuccessCode,
						},
						CommandID:  "command-id-1",
						AccountID:  "owner",
						InstanceID: "instance-id-1",
						Region:     "eu-central-1",
						ExitCode:   0,
						Status:     string(ssmtypes.CommandInvocationStatusSuccess),
					}, ae)
				},
			},
			staticMatchers:         defaultStaticMatcher,
			wantInstalledInstances: []string{"instance-id-1"},
		},
		{
			name: "nodes present, instance filtered",
			presentInstances: []types.Server{
				&types.ServerV2{
					Kind: types.KindNode,
					Metadata: types.Metadata{
						Name: "name",
						Labels: map[string]string{
							types.AWSAccountIDLabel:  "owner",
							types.AWSInstanceIDLabel: "instance-id-1",
						},
						Namespace: defaults.Namespace,
					},
				},
			},
			foundEC2Instances: []ec2types.Instance{
				{
					InstanceId: aws.String("instance-id-1"),
					Tags: []ec2types.Tag{{
						Key:   aws.String("env"),
						Value: aws.String("dev"),
					}},
					State: &ec2types.InstanceState{
						Name: ec2types.InstanceStateNameRunning,
					},
				},
			},
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			staticMatchers: defaultStaticMatcher,
			emitter:        &mockEmitter{},
		},
		{
			name: "nodes present, instance not filtered",
			presentInstances: []types.Server{
				&types.ServerV2{
					Kind: types.KindNode,
					Metadata: types.Metadata{
						Name: "name",
						Labels: map[string]string{
							types.AWSAccountIDLabel:  "owner",
							types.AWSInstanceIDLabel: "wow-its-a-different-instance",
						},
						Namespace: defaults.Namespace,
					},
				},
			},
			foundEC2Instances: []ec2types.Instance{
				{
					InstanceId: aws.String("instance-id-1"),
					Tags: []ec2types.Tag{{
						Key:   aws.String("env"),
						Value: aws.String("dev"),
					}},
					State: &ec2types.InstanceState{
						Name: ec2types.InstanceStateNameRunning,
					},
				},
			},
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			emitter:                &mockEmitter{},
			staticMatchers:         defaultStaticMatcher,
			wantInstalledInstances: []string{"instance-id-1"},
		},
		{
			name:              "chunked nodes get 2 log messages",
			presentInstances:  []types.Server{},
			foundEC2Instances: genEC2Instances(58),
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			emitter:                &mockEmitter{},
			staticMatchers:         defaultStaticMatcher,
			wantInstalledInstances: genEC2InstanceIDs(58),
		},
		{
			name:             "no nodes present, 1 found using dynamic matchers",
			presentInstances: []types.Server{},
			foundEC2Instances: []ec2types.Instance{
				{
					InstanceId: aws.String("instance-id-1"),
					Tags: []ec2types.Tag{{
						Key:   aws.String("env"),
						Value: aws.String("dev"),
					}},
					State: &ec2types.InstanceState{
						Name: ec2types.InstanceStateNameRunning,
					},
				},
			},
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			emitter: &mockEmitter{
				eventHandler: func(t *testing.T, ae events.AuditEvent, server *Server) {
					t.Helper()
					require.Equal(t, &events.SSMRun{
						Metadata: events.Metadata{
							Type: libevents.SSMRunEvent,
							Code: libevents.SSMRunSuccessCode,
						},
						CommandID:  "command-id-1",
						AccountID:  "owner",
						InstanceID: "instance-id-1",
						Region:     "eu-central-1",
						ExitCode:   0,
						Status:     string(ssmtypes.CommandInvocationStatusSuccess),
					}, ae)
				},
			},
			staticMatchers:         Matchers{},
			discoveryConfig:        defaultDiscoveryConfig,
			wantInstalledInstances: []string{"instance-id-1"},
		},
		{
			name:             "one node found with Script mode using Integration credentials",
			presentInstances: []types.Server{},
			foundEC2Instances: []ec2types.Instance{
				{
					InstanceId: aws.String("instance-id-1"),
					Tags: []ec2types.Tag{{
						Key:   aws.String("env"),
						Value: aws.String("dev"),
					}},
					State: &ec2types.InstanceState{
						Name: ec2types.InstanceStateNameRunning,
					},
				},
			},
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			emitter: &mockEmitter{
				eventHandler: func(t *testing.T, ae events.AuditEvent, server *Server) {
					t.Helper()
					require.Equal(t, &events.SSMRun{
						Metadata: events.Metadata{
							Type: libevents.SSMRunEvent,
							Code: libevents.SSMRunSuccessCode,
						},
						CommandID:  "command-id-1",
						AccountID:  "owner",
						InstanceID: "instance-id-1",
						Region:     "eu-central-1",
						ExitCode:   0,
						Status:     string(ssmtypes.CommandInvocationStatusSuccess),
					}, ae)
				},
			},
			staticMatchers:  Matchers{},
			discoveryConfig: dcForEC2SSMWithIntegration,
			wantDiscoveryConfigStatus: &discoveryconfig.Status{
				State:               "DISCOVERY_CONFIG_STATE_SYNCING",
				ErrorMessage:        nil,
				DiscoveredResources: 1,
				LastSyncTime:        fakeClock.Now().UTC(),
				IntegrationDiscoveredResources: map[string]*discoveryconfigv1.IntegrationDiscoveredSummary{
					"my-integration": {
						AwsEc2: &discoveryconfigv1.ResourcesDiscoveredSummary{
							Found:    1,
							Enrolled: 0,
							Failed:   0,
						},
					},
				},
			},
			wantInstalledInstances: []string{"instance-id-1"},
		},
		{
			name:              "no nodes found using DiscoveryConfig and Integration, but DiscoveryConfig Status is still updated",
			presentInstances:  []types.Server{},
			foundEC2Instances: []ec2types.Instance{},
			ssm:               &mockSSMClient{},
			emitter:           &mockEmitter{},
			staticMatchers:    Matchers{},
			discoveryConfig:   dcForEC2StatusWithoutMatch,
			wantDiscoveryConfigStatus: &discoveryconfig.Status{
				State:               "DISCOVERY_CONFIG_STATE_SYNCING",
				ErrorMessage:        nil,
				DiscoveredResources: 0,
				LastSyncTime:        fakeClock.Now().UTC(),
				IntegrationDiscoveredResources: map[string]*discoveryconfigv1.IntegrationDiscoveredSummary{
					"my-integration": {
						AwsEc2: &discoveryconfigv1.ResourcesDiscoveredSummary{
							Found:    0,
							Enrolled: 0,
							Failed:   0,
						},
					},
				},
			},
			wantInstalledInstances: []string{},
		},
		{
			name:             "one node found but SSM Run fails and DiscoverEC2 User Task is created",
			presentInstances: []types.Server{},
			foundEC2Instances: []ec2types.Instance{
				{
					InstanceId: aws.String("instance-id-1"),
					Tags: []ec2types.Tag{{
						Key:   aws.String("env"),
						Value: aws.String("dev"),
					}},
					State: &ec2types.InstanceState{
						Name: ec2types.InstanceStateNameRunning,
					},
				},
			},
			ssm: &mockSSMClient{
				commandOutput: &ssm.SendCommandOutput{
					Command: &ssmtypes.Command{
						CommandId: aws.String("command-id-1"),
					},
				},
				invokeOutput: &ssm.GetCommandInvocationOutput{
					Status:       ssmtypes.CommandInvocationStatusSuccess,
					ResponseCode: 0,
				},
			},
			ssmRunError: trace.BadParameter("ssm run failed"),
			emitter: &mockEmitter{
				eventHandler: func(t *testing.T, ae events.AuditEvent, server *Server) {
					t.Helper()
					require.Equal(t, &events.SSMRun{
						Metadata: events.Metadata{
							Type: libevents.SSMRunEvent,
							Code: libevents.SSMRunSuccessCode,
						},
						CommandID:  "command-id-1",
						AccountID:  "owner",
						InstanceID: "instance-id-1",
						Region:     "eu-central-1",
						ExitCode:   0,
						Status:     string(ssmtypes.CommandInvocationStatusSuccess),
					}, ae)
				},
			},
			staticMatchers:         Matchers{},
			discoveryConfig:        discoveryConfigForUserTaskEC2Test,
			wantInstalledInstances: []string{},
			userTasksDiscoverCheck: func(t *testing.T, userTasksClt services.UserTasks) {
				atLeastOneUserTask := 1
				existingTasks := fetchAllUserTasks(t, userTasksClt, atLeastOneUserTask, 0)
				existingTask := existingTasks[0]

				require.Equal(t, "OPEN", existingTask.GetSpec().State)
				require.Equal(t, "my-integration", existingTask.GetSpec().Integration)
				require.Equal(t, "ec2-ssm-invocation-failure", existingTask.GetSpec().IssueType)
				require.Equal(t, "owner", existingTask.GetSpec().GetDiscoverEc2().GetAccountId())
				require.Equal(t, "eu-west-2", existingTask.GetSpec().GetDiscoverEc2().GetRegion())

				taskInstances := existingTask.GetSpec().GetDiscoverEc2().Instances
				require.Contains(t, taskInstances, "instance-id-1")
				taskInstance := taskInstances["instance-id-1"]

				require.Equal(t, "instance-id-1", taskInstance.InstanceId)
				require.Equal(t, discoveryConfigForUserTaskEC2TestName, taskInstance.DiscoveryConfig)
				require.Equal(t, defaultDiscoveryGroup, taskInstance.DiscoveryGroup)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			ec2Client := &mockEC2Client{
				output: &ec2.DescribeInstancesOutput{
					Reservations: []ec2types.Reservation{
						{
							OwnerId:   aws.String("owner"),
							Instances: tc.foundEC2Instances,
						},
					},
				},
			}

			// Create and start test auth server.
			testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
				Dir: t.TempDir(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

			awsOIDCIntegration, err := types.NewIntegrationAWSOIDC(types.Metadata{
				Name: "my-integration",
			}, &types.AWSOIDCIntegrationSpecV1{
				RoleARN: "arn:aws:iam::123456789012:role/teleport",
			})
			require.NoError(t, err)
			testAuthServer.AuthServer.IntegrationsTokenGenerator = &mockIntegrationsTokenGenerator{
				proxies: nil,
				integrations: map[string]types.Integration{
					awsOIDCIntegration.GetName(): awsOIDCIntegration,
				},
			}

			tlsServer, err := testAuthServer.NewTestTLSServer()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })
			_, err = tlsServer.Auth().CreateIntegration(ctx, awsOIDCIntegration)
			require.NoError(t, err)

			// Auth client for discovery service.
			identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
			authClient, err := tlsServer.NewClient(identity)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, authClient.Close()) })

			for _, instance := range tc.presentInstances {
				_, err := tlsServer.Auth().UpsertNode(ctx, instance)
				require.NoError(t, err)
			}

			logger := logtest.NewLogger()

			reporter := &mockUsageReporter{}
			installer := &mockSSMInstaller{
				installedInstances: make(map[string]struct{}),
				runError:           tc.ssmRunError,
			}
			tlsServer.Auth().SetUsageReporter(reporter)

			if tc.discoveryConfig != nil {
				_, err := tlsServer.Auth().DiscoveryConfigs.CreateDiscoveryConfig(ctx, tc.discoveryConfig)
				require.NoError(t, err)
			}

			fakeConfigProvider := mocks.AWSConfigProvider{
				OIDCIntegrationClient: tlsServer.Auth(),
			}
			server, err := New(authz.ContextWithUser(context.Background(), identity.I), &Config{
				GetEC2Client: func(ctx context.Context, region string, opts ...awsconfig.OptionsFn) (ec2.DescribeInstancesAPIClient, error) {
					return ec2Client, nil
				},
				GetSSMClient: func(ctx context.Context, region string, opts ...awsconfig.OptionsFn) (server.SSMClient, error) {
					return tc.ssm, nil
				},
				AWSConfigProvider: &fakeConfigProvider,
				AWSFetchersClients: &mockFetchersClients{
					AWSConfigProvider: fakeConfigProvider,
				},
				ClusterFeatures:  func() proto.Features { return proto.Features{} },
				KubernetesClient: fake.NewSimpleClientset(),
				AccessPoint:      getDiscoveryAccessPointWithEKSEnroller(tlsServer.Auth(), authClient, authClient.IntegrationAWSOIDCClient()),
				Matchers:         tc.staticMatchers,
				Emitter:          tc.emitter,
				Log:              logger,
				DiscoveryGroup:   defaultDiscoveryGroup,
				clock:            fakeClock,
			})
			require.NoError(t, err)
			server.ec2Installer = installer
			tc.emitter.server = server
			tc.emitter.t = t

			go server.Start()
			t.Cleanup(server.Stop)

			if len(tc.wantInstalledInstances) > 0 {
				slices.Sort(tc.wantInstalledInstances)
				require.Eventually(t, func() bool {
					instances := installer.GetInstalledInstances()
					slices.Sort(instances)
					return slices.Equal(tc.wantInstalledInstances, instances) && len(tc.wantInstalledInstances) == reporter.ResourceCreateEventCount()
				}, 10*time.Second, 50*time.Millisecond)
			} else {
				require.Never(t, func() bool {
					return len(installer.GetInstalledInstances()) > 0 || reporter.ResourceCreateEventCount() > 0
				}, 500*time.Millisecond, 50*time.Millisecond)
			}
			require.Eventually(t, func() bool {
				return reporter.DiscoveryFetchEventCount() > 0
			}, 10*time.Second, 50*time.Millisecond)

			// Discovery Config Status is updated accordingly
			if tc.wantDiscoveryConfigStatus != nil {
				// It can take a while for the status to be updated.
				require.Eventually(t, func() bool {
					fakeClock.Advance(server.PollInterval)
					storedDiscoveryConfig, err := tlsServer.Auth().DiscoveryConfigs.GetDiscoveryConfig(ctx, tc.discoveryConfig.GetName())
					require.NoError(t, err)
					if len(storedDiscoveryConfig.Status.IntegrationDiscoveredResources) == 0 {
						return false
					}
					want := *tc.wantDiscoveryConfigStatus
					got := storedDiscoveryConfig.Status

					require.Equal(t, want.State, got.State)
					require.Equal(t, want.DiscoveredResources, got.DiscoveredResources)
					require.Equal(t, want.ErrorMessage, got.ErrorMessage)
					for expectedKey, expectedValue := range want.IntegrationDiscoveredResources {
						require.Contains(t, got.IntegrationDiscoveredResources, expectedKey)
						require.Equal(t, expectedValue, got.IntegrationDiscoveredResources[expectedKey])
					}
					return true
				}, 1*time.Second, 50*time.Millisecond)
			}
			if tc.userTasksDiscoverCheck != nil {
				tc.userTasksDiscoverCheck(t, tlsServer.Auth().UserTasks)
			}
		})
	}
}

func fetchAllUserTasks(t *testing.T, userTasksClt services.UserTasks, minUserTasks, minUserTaskResources int) []*usertasksv1.UserTask {
	var existingTasks []*usertasksv1.UserTask
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		var allTasks []*usertasksv1.UserTask
		var nextToken string
		for {
			var userTasks []*usertasksv1.UserTask
			userTasks, nextTokenResp, err := userTasksClt.ListUserTasks(context.Background(), 0, nextToken, &usertasksv1.ListUserTasksFilters{})
			assert.NoError(t, err)
			allTasks = append(allTasks, userTasks...)
			if nextTokenResp == "" {
				break
			}
			nextToken = nextTokenResp
		}
		existingTasks = allTasks

		if !assert.GreaterOrEqual(t, len(allTasks), minUserTasks) {
			return
		}

		gotResources := 0
		for _, task := range allTasks {
			gotResources += len(task.GetSpec().GetDiscoverEc2().GetInstances())
			gotResources += len(task.GetSpec().GetDiscoverEks().GetClusters())
			gotResources += len(task.GetSpec().GetDiscoverRds().GetDatabases())
		}
		assert.GreaterOrEqual(t, gotResources, minUserTaskResources)
	}, 10*time.Second, 50*time.Millisecond)

	return existingTasks
}

func TestDiscoveryServerConcurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := logtest.NewLogger()

	defaultDiscoveryGroup := "dg01"
	awsMatcher := types.AWSMatcher{
		Types:       []string{"ec2"},
		Regions:     []string{"eu-central-1"},
		Tags:        map[string]utils.Strings{"teleport": {"yes"}},
		Integration: "my-integration",
		SSM:         &types.AWSSSM{DocumentName: "document"},
	}
	require.NoError(t, awsMatcher.CheckAndSetDefaults())
	staticMatcher := Matchers{
		AWS: []types.AWSMatcher{awsMatcher},
	}

	emitter := &mockEmitter{
		eventHandler: func(t *testing.T, ae events.AuditEvent, server *Server) {
			t.Helper()
		},
	}

	testCloudClients := &cloud.TestCloudClients{}

	ec2Client := &mockEC2Client{
		output: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{
				{
					OwnerId: aws.String("123456789012"),
					Instances: []ec2types.Instance{
						{
							InstanceId: aws.String("i-123456789012"),
							Tags: []ec2types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("dev"),
								},
							},
							PrivateIpAddress: aws.String("172.0.1.2"),
							VpcId:            aws.String("vpcId"),
							SubnetId:         aws.String("subnetId"),
							PrivateDnsName:   aws.String("privateDnsName"),
							State: &ec2types.InstanceState{
								Name: ec2types.InstanceStateNameRunning,
							},
						},
					},
				},
			},
		},
	}

	// Create and start test auth server.
	testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
		Dir: t.TempDir(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

	tlsServer, err := testAuthServer.NewTestTLSServer()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

	// Auth client for discovery service.
	identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
	authClient, err := tlsServer.NewClient(identity)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authClient.Close()) })

	getEC2Client := func(ctx context.Context, region string, opts ...awsconfig.OptionsFn) (ec2.DescribeInstancesAPIClient, error) {
		return ec2Client, nil
	}

	// Create Server1
	server1, err := New(authz.ContextWithUser(ctx, identity.I), &Config{
		CloudClients:     testCloudClients,
		GetEC2Client:     getEC2Client,
		ClusterFeatures:  func() proto.Features { return proto.Features{} },
		KubernetesClient: fake.NewSimpleClientset(),
		AccessPoint:      getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
		Matchers:         staticMatcher,
		Emitter:          emitter,
		Log:              logger,
		DiscoveryGroup:   defaultDiscoveryGroup,
	})
	require.NoError(t, err)

	// Create Server2
	server2, err := New(authz.ContextWithUser(ctx, identity.I), &Config{
		CloudClients:     testCloudClients,
		GetEC2Client:     getEC2Client,
		ClusterFeatures:  func() proto.Features { return proto.Features{} },
		KubernetesClient: fake.NewSimpleClientset(),
		AccessPoint:      getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
		Matchers:         staticMatcher,
		Emitter:          emitter,
		Log:              logger,
		DiscoveryGroup:   defaultDiscoveryGroup,
	})
	require.NoError(t, err)

	// Start both servers.
	go server1.Start()
	t.Cleanup(server1.Stop)

	go server2.Start()
	t.Cleanup(server2.Stop)

	// We must get only one EC2 EICE Node.
	// Even when two servers are discovering the same EC2 Instance, they will use the same name when converting to EICE Node.
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		allNodes, err := tlsServer.Auth().GetNodes(ctx, "default")
		assert.NoError(t, err)
		assert.Len(t, allNodes, 1)
	}, 1*time.Second, 50*time.Millisecond)

	// We should never get a duplicate instance.
	require.Never(t, func() bool {
		allNodes, err := tlsServer.Auth().GetNodes(ctx, "default")
		require.NoError(t, err)

		return len(allNodes) != 1
	}, 2*time.Second, 50*time.Millisecond)
}

func newMockKubeService(name, namespace, externalName string, labels, annotations map[string]string, ports []corev1.ServicePort) *corev1.Service {
	serviceType := corev1.ServiceTypeClusterIP
	if externalName != "" {
		serviceType = corev1.ServiceTypeExternalName
	}
	return &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports:        ports,
			ClusterIP:    "192.168.100.100",
			ClusterIPs:   []string{"192.168.100.100"},
			Type:         serviceType,
			ExternalName: externalName,
		},
	}
}

type noopProtocolChecker struct{}

// CheckProtocol for noopProtocolChecker just returns 'tcp'
func (*noopProtocolChecker) CheckProtocol(service corev1.Service, port corev1.ServicePort) string {
	return "tcp"
}

func TestDiscoveryKubeServices(t *testing.T) {
	const (
		mainDiscoveryGroup  = "main"
		otherDiscoveryGroup = "other"
	)
	t.Parallel()

	appProtocolHTTP := "http"
	mockKubeServices := []*corev1.Service{
		newMockKubeService("service1", "ns1", "",
			map[string]string{"test-label": "testval"},
			map[string]string{types.DiscoveryPublicAddr: "custom.example.com", types.DiscoveryPathLabel: "foo/bar baz"},
			[]corev1.ServicePort{{Port: 42, Name: "http", Protocol: corev1.ProtocolTCP}}),
		newMockKubeService("service2", "ns2", "",
			map[string]string{
				"test-label":  "testval",
				"test-label2": "testval2",
			},
			nil,
			[]corev1.ServicePort{{Port: 42, Name: "custom", AppProtocol: &appProtocolHTTP, Protocol: corev1.ProtocolTCP}}),
	}

	app1 := mustConvertKubeServiceToApp(t, mainDiscoveryGroup, "http", mockKubeServices[0], mockKubeServices[0].Spec.Ports[0])
	modifiedApp1 := mustConvertKubeServiceToApp(t, mainDiscoveryGroup, "http", mockKubeServices[0], mockKubeServices[0].Spec.Ports[0])
	modifiedApp1.SetURI("http://wrong.example.com")
	app2 := mustConvertKubeServiceToApp(t, mainDiscoveryGroup, "http", mockKubeServices[1], mockKubeServices[1].Spec.Ports[0])
	otherGroupApp1 := mustConvertKubeServiceToApp(t, otherDiscoveryGroup, "http", mockKubeServices[0], mockKubeServices[0].Spec.Ports[0])

	tests := []struct {
		name                      string
		existingApps              []types.Application
		kubernetesMatchers        []types.KubernetesMatcher
		expectedAppsToExistInAuth []types.Application
	}{
		{
			name: "no apps in auth server, import 2 apps",
			kubernetesMatchers: []types.KubernetesMatcher{
				{
					Types:      []string{"app"},
					Namespaces: []string{types.Wildcard},
					Labels:     map[string]utils.Strings{"test-label": {"testval"}},
				},
			},
			expectedAppsToExistInAuth: types.Apps{app1.Copy(), app2.Copy()},
		},
		{
			name:         "one app in auth server, import 1 apps",
			existingApps: types.Apps{app1.Copy()},
			kubernetesMatchers: []types.KubernetesMatcher{
				{
					Types:      []string{"app"},
					Namespaces: []string{types.Wildcard},
					Labels:     map[string]utils.Strings{"test-label": {"testval"}},
				},
			},
			expectedAppsToExistInAuth: types.Apps{app1.Copy(), app2.Copy()},
		},
		{
			name:         "two apps in the auth server, one updated one imported",
			existingApps: types.Apps{modifiedApp1.Copy(), app2.Copy()},
			kubernetesMatchers: []types.KubernetesMatcher{
				{
					Types:      []string{"app"},
					Namespaces: []string{types.Wildcard},
					Labels:     map[string]utils.Strings{"test-label": {"testval"}},
				},
			},
			expectedAppsToExistInAuth: types.Apps{app1.Copy(), app2.Copy()},
		},
		{
			name:         "one app in auth server, discovery doesn't match another app",
			existingApps: types.Apps{app1.Copy()},
			kubernetesMatchers: []types.KubernetesMatcher{
				{
					Types:      []string{"app"},
					Namespaces: []string{"ns1"},
					Labels:     map[string]utils.Strings{"test-label": {"testval"}},
				},
			},
			expectedAppsToExistInAuth: types.Apps{app1.Copy()},
		},
		{
			name:         "one app in auth server from another discovery group, import 2 apps",
			existingApps: types.Apps{otherGroupApp1.Copy()},
			kubernetesMatchers: []types.KubernetesMatcher{
				{
					Types:      []string{"app"},
					Namespaces: []string{types.Wildcard},
					Labels:     map[string]utils.Strings{"test-label": {"testval"}},
				},
			},
			expectedAppsToExistInAuth: types.Apps{app1.Copy(), otherGroupApp1.Copy(), app2.Copy()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var objects []runtime.Object
			for _, s := range mockKubeServices {
				objects = append(objects, s)
			}

			ctx := context.Background()
			// Create and start test auth server.
			testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
				Dir: t.TempDir(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

			tlsServer, err := testAuthServer.NewTestTLSServer()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

			// Auth client for discovery service.
			authClient, err := tlsServer.NewClient(authtest.TestServerID(types.RoleDiscovery, "hostID"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, authClient.Close()) })

			for _, app := range tt.existingApps {
				err := tlsServer.Auth().CreateApp(ctx, app)
				require.NoError(t, err)
			}

			require.Eventually(t, func() bool {
				existingApps, err := tlsServer.Auth().GetApps(ctx)
				return err == nil && len(existingApps) == len(tt.existingApps)
			}, time.Second, 100*time.Millisecond)

			discServer, err := New(
				ctx,
				&Config{
					CloudClients:     &cloud.TestCloudClients{},
					ClusterFeatures:  func() proto.Features { return proto.Features{} },
					KubernetesClient: fake.NewSimpleClientset(objects...),
					AccessPoint:      getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
					Matchers: Matchers{
						Kubernetes: tt.kubernetesMatchers,
					},
					Emitter:         authClient,
					DiscoveryGroup:  mainDiscoveryGroup,
					protocolChecker: &noopProtocolChecker{},
				})

			require.NoError(t, err)

			t.Cleanup(func() {
				discServer.Stop()
			})
			go discServer.Start()

			require.EventuallyWithT(t, func(t *assert.CollectT) {
				existingApps, err := tlsServer.Auth().GetApps(ctx)
				if !assert.NoError(t, err) {
					return
				}
				if !assert.Len(t, existingApps, len(tt.expectedAppsToExistInAuth)) {
					return
				}
				a1 := types.Apps(existingApps)
				a2 := types.Apps(tt.expectedAppsToExistInAuth)
				for k := range a1 {
					if !assert.Equal(t, services.Equal, services.CompareResources(a1[k], a2[k])) {
						return
					}
				}
			}, 5*time.Second, 200*time.Millisecond)
		})
	}
}

func TestDiscoveryInCloudKube(t *testing.T) {
	t.Parallel()

	const (
		mainDiscoveryGroup  = "main"
		otherDiscoveryGroup = "other"
	)
	tcs := []struct {
		name                          string
		existingKubeClusters          []types.KubeCluster
		awsMatchers                   []types.AWSMatcher
		azureMatchers                 []types.AzureMatcher
		gcpMatchers                   []types.GCPMatcher
		expectedClustersToExistInAuth []types.KubeCluster
		clustersNotUpdated            []string
		expectedAssumedRoles          []string
		expectedExternalIDs           []string
		wantEvents                    int
	}{
		{
			name:                 "no clusters in auth server, import 2 prod clusters from EKS",
			existingKubeClusters: []types.KubeCluster{},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			wantEvents: 2,
		},
		{
			name:                 "no clusters in auth server, import 2 prod clusters from EKS with assumed roles",
			existingKubeClusters: []types.KubeCluster{},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
					AssumeRole: &types.AssumeRole{
						RoleARN:    "arn:aws:iam::123456789012:role/teleport-role",
						ExternalID: "external-id",
					},
				},
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
					AssumeRole: &types.AssumeRole{
						RoleARN:    "arn:aws:iam::123456789012:role/teleport-role2",
						ExternalID: "external-id2",
					},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			expectedAssumedRoles: []string{"arn:aws:iam::123456789012:role/teleport-role", "arn:aws:iam::123456789012:role/teleport-role2"},
			expectedExternalIDs:  []string{"external-id", "external-id2"},
			wantEvents:           2,
		},
		{
			name:                 "no clusters in auth server, import 2 stg clusters from EKS",
			existingKubeClusters: []types.KubeCluster{},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"stg"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[2], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[3], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			wantEvents: 2,
		},
		{
			name: "1 cluster in auth server not updated + import 1 prod cluster from EKS",
			existingKubeClusters: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			clustersNotUpdated: []string{mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}).GetName()},
			wantEvents:         1,
		},
		{
			name: "1 cluster in auth that belongs the same discovery group but has unmatched labels + import 2 prod clusters from EKS",
			existingKubeClusters: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[3], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			clustersNotUpdated: []string{},
			wantEvents:         2,
		},
		{
			name: "1 cluster in auth that belongs to a different discovery group + import 2 prod clusters from EKS",
			existingKubeClusters: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[3], rewriteDiscoveryLabelsParams{discoveryGroup: otherDiscoveryGroup}),
			},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[3], rewriteDiscoveryLabelsParams{discoveryGroup: otherDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			clustersNotUpdated: []string{},
			wantEvents:         2,
		},
		{
			name: "1 cluster in auth that must be updated + import 1 prod clusters from EKS",
			existingKubeClusters: []types.KubeCluster{
				// add an extra static label to force update in auth server
				modifyKubeCluster(mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup})),
			},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			clustersNotUpdated: []string{},
			wantEvents:         1,
		},
		{
			name: "2 clusters in auth that matches but one must be updated +  import 2 prod clusters, 1 from EKS and other from AKS",
			existingKubeClusters: []types.KubeCluster{
				// add an extra static label to force update in auth server
				modifyKubeCluster(mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup})),
				mustConvertAKSToKubeCluster(t, aksMockClusters["group1"][0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			awsMatchers: []types.AWSMatcher{
				{
					Types:   []string{"eks"},
					Regions: []string{"eu-west-1"},
					Tags:    map[string]utils.Strings{"env": {"prod"}},
				},
			},
			azureMatchers: []types.AzureMatcher{
				{
					Types:          []string{"aks"},
					ResourceTags:   map[string]utils.Strings{"env": {"prod"}},
					Regions:        []string{types.Wildcard},
					ResourceGroups: []string{types.Wildcard},
					Subscriptions:  []string{"sub1"},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertEKSToKubeCluster(t, eksMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertAKSToKubeCluster(t, aksMockClusters["group1"][0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertAKSToKubeCluster(t, aksMockClusters["group1"][1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			clustersNotUpdated: []string{mustConvertAKSToKubeCluster(t, aksMockClusters["group1"][0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}).GetName()},
			wantEvents:         2,
		},
		{
			name:                 "no clusters in auth server, import 2 prod clusters from GKE",
			existingKubeClusters: []types.KubeCluster{},
			gcpMatchers: []types.GCPMatcher{
				{
					Types:      []string{"gke"},
					Locations:  []string{"*"},
					ProjectIDs: []string{"p1"},
					Tags:       map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertGKEToKubeCluster(t, gkeMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertGKEToKubeCluster(t, gkeMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			wantEvents: 2,
		},
		{
			name:                 "no clusters in auth server, import 3 prod clusters from GKE across multiple projects",
			existingKubeClusters: []types.KubeCluster{},
			gcpMatchers: []types.GCPMatcher{
				{
					Types:      []string{"gke"},
					Locations:  []string{"*"},
					ProjectIDs: []string{"*"},
					Tags:       map[string]utils.Strings{"env": {"prod"}},
				},
			},
			expectedClustersToExistInAuth: []types.KubeCluster{
				mustConvertGKEToKubeCluster(t, gkeMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertGKEToKubeCluster(t, gkeMockClusters[1], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
				mustConvertGKEToKubeCluster(t, gkeMockClusters[4], rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup}),
			},
			wantEvents: 3,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testCloudClients := &cloud.TestCloudClients{
				AzureAKSClient: newPopulatedAKSMock(),
				GCPGKE:         newPopulatedGCPMock(),
				GCPProjects:    newPopulatedGCPProjectsMock(),
			}

			ctx := context.Background()
			// Create and start test auth server.
			testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
				Dir: t.TempDir(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

			tlsServer, err := testAuthServer.NewTestTLSServer()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

			// Auth client for discovery service.
			identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
			authClient, err := tlsServer.NewClient(identity)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, authClient.Close()) })

			for _, kubeCluster := range tc.existingKubeClusters {
				err := tlsServer.Auth().CreateKubernetesCluster(ctx, kubeCluster)
				require.NoError(t, err)
			}
			// We analyze the logs emitted by discovery service to detect clusters that were not updated
			// because their state didn't change.
			r, w := io.Pipe()
			t.Cleanup(func() {
				require.NoError(t, r.Close())
				require.NoError(t, w.Close())
			})
			logger := logtest.NewLogger()

			clustersNotUpdated := make(chan string, 10)
			go func() {
				// reconcileRegexp is the regex extractor of a log message emitted by reconciler when
				// the current state of the cluster is equal to the previous.
				// [r.log.Debugf("%v %v is already registered.", new.GetKind(), new.GetName())]
				// lib/services/reconciler.go
				reconcileRegexp := regexp.MustCompile("kube_cluster (.*) is already registered")

				scanner := bufio.NewScanner(r)
				for scanner.Scan() {
					text := scanner.Text()
					// we analyze the logs emitted by discovery service to detect clusters that were not updated
					// because their state didn't change.
					if reconcileRegexp.MatchString(text) {
						result := reconcileRegexp.FindStringSubmatch(text)
						if len(result) != 2 {
							continue
						}
						clustersNotUpdated <- result[1]
					}
				}
			}()

			reporter := &mockUsageReporter{}
			tlsServer.Auth().SetUsageReporter(reporter)

			mockedClients := &mockFetchersClients{
				AWSConfigProvider: mocks.AWSConfigProvider{
					STSClient:             &mocks.STSClient{},
					OIDCIntegrationClient: newFakeAccessPoint(),
				},
				eksClusters: newPopulatedEKSMock().clusters,
			}

			discServer, err := New(
				authz.ContextWithUser(ctx, identity.I),
				&Config{
					CloudClients:       testCloudClients,
					AWSFetchersClients: mockedClients,
					ClusterFeatures:    func() proto.Features { return proto.Features{} },
					KubernetesClient:   fake.NewSimpleClientset(),
					AccessPoint:        getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
					Matchers: Matchers{
						AWS:   tc.awsMatchers,
						Azure: tc.azureMatchers,
						GCP:   tc.gcpMatchers,
					},
					Emitter:        authClient,
					Log:            logger,
					DiscoveryGroup: mainDiscoveryGroup,
				})
			require.NoError(t, err)

			t.Cleanup(discServer.Stop)
			go discServer.Start()

			clustersNotUpdatedMap := sliceToSet(tc.clustersNotUpdated)
			clustersFoundInAuth := false
			require.Eventually(t, func() bool {
			loop:
				for {
					select {
					case cluster := <-clustersNotUpdated:
						if _, ok := clustersNotUpdatedMap[cluster]; !ok {
							require.Failf(t, "expected Action for cluster %s but got no action from reconciler", cluster)
						}
						delete(clustersNotUpdatedMap, cluster)
					default:
						kubeClusters, err := tlsServer.Auth().GetKubernetesClusters(ctx)
						require.NoError(t, err)
						if len(kubeClusters) == len(tc.expectedClustersToExistInAuth) {
							c1 := types.KubeClusters(kubeClusters).ToMap()
							c2 := types.KubeClusters(tc.expectedClustersToExistInAuth).ToMap()
							for k := range c1 {
								if services.CompareResources(c1[k], c2[k]) != services.Equal {
									return false
								}
							}
							clustersFoundInAuth = true
						}
						break loop
					}
				}
				return len(clustersNotUpdated) == 0 && clustersFoundInAuth
			}, 5*time.Second, 200*time.Millisecond)

			require.ElementsMatch(t, tc.expectedAssumedRoles, mockedClients.STSClient.GetAssumedRoleARNs(), "roles incorrectly assumed")
			require.ElementsMatch(t, tc.expectedExternalIDs, mockedClients.STSClient.GetAssumedRoleExternalIDs(), "external IDs incorrectly assumed")

			if tc.wantEvents > 0 {
				require.Eventually(t, func() bool {
					return reporter.ResourceCreateEventCount() == tc.wantEvents
				}, time.Second, 100*time.Millisecond)
			} else {
				require.Never(t, func() bool {
					return reporter.ResourceCreateEventCount() != 0
				}, time.Second, 100*time.Millisecond)
			}
		})
	}
}

func TestDiscoveryServer_New(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		desc                string
		cloudClients        fetchers.AWSClientGetter
		matchers            Matchers
		errAssertion        require.ErrorAssertionFunc
		discServerAssertion require.ValueAssertionFunc
	}{
		{
			desc: "no matchers error",

			cloudClients: &mockFetchersClients{},
			matchers:     Matchers{},
			errAssertion: func(t require.TestingT, err error, i ...any) {
				require.ErrorIs(t, err, &trace.BadParameterError{Message: "no matchers or discovery group configured for discovery"})
			},
			discServerAssertion: require.Nil,
		},
		{
			desc: "success with EKS matcher",

			cloudClients: &mockFetchersClients{},

			matchers: Matchers{
				AWS: []types.AWSMatcher{
					{
						Types:   []string{"eks"},
						Regions: []string{"eu-west-1"},
						Tags:    map[string]utils.Strings{"env": {"prod"}},
						AssumeRole: &types.AssumeRole{
							RoleARN:    "arn:aws:iam::123456789012:role/teleport-role",
							ExternalID: "external-id",
						},
					},
				},
			},
			errAssertion: require.NoError,
			discServerAssertion: func(t require.TestingT, i any, i2 ...any) {
				require.NotNil(t, i)
				val, ok := i.(*Server)
				require.True(t, ok)
				require.Len(t, val.kubeFetchers, 1, "unexpected amount of kube fetchers")
			},
		},
		{
			desc:         "EKS fetcher is skipped on initialization error (missing region)",
			cloudClients: &mockFetchersClients{},
			matchers: Matchers{
				AWS: []types.AWSMatcher{
					{
						Types:   []string{"eks"},
						Regions: []string{},
						Tags:    map[string]utils.Strings{"env": {"prod"}},
						AssumeRole: &types.AssumeRole{
							RoleARN:    "arn:aws:iam::123456789012:role/teleport-role",
							ExternalID: "external-id",
						},
					},
					{
						Types:   []string{"eks"},
						Regions: []string{"eu-west-1"},
						Tags:    map[string]utils.Strings{"env": {"staging"}},
						AssumeRole: &types.AssumeRole{
							RoleARN:    "arn:aws:iam::55555555555:role/teleport-role",
							ExternalID: "external-id2",
						},
					},
				},
			},
			errAssertion: require.NoError,
			discServerAssertion: func(t require.TestingT, i any, i2 ...any) {
				require.NotNil(t, i)
				val, ok := i.(*Server)
				require.True(t, ok)
				require.Len(t, val.kubeFetchers, 1, "unexpected amount of kube fetchers")
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := t.Context()

			discServer, err := New(
				ctx,
				&Config{
					AWSFetchersClients: tt.cloudClients,
					ClusterFeatures:    func() proto.Features { return proto.Features{} },
					AccessPoint:        newFakeAccessPoint(),
					Matchers:           tt.matchers,
					Emitter:            &mockEmitter{},
					protocolChecker:    &noopProtocolChecker{},
				})

			tt.errAssertion(t, err)
			tt.discServerAssertion(t, discServer)
		})
	}
}

type mockAKSAPI struct {
	azure.AKSClient
	group map[string][]*azure.AKSCluster
}

func (m *mockAKSAPI) ListAll(ctx context.Context) ([]*azure.AKSCluster, error) {
	result := make([]*azure.AKSCluster, 0, 10)
	for _, v := range m.group {
		result = append(result, v...)
	}
	return result, nil
}

func (m *mockAKSAPI) ListWithinGroup(ctx context.Context, group string) ([]*azure.AKSCluster, error) {
	return m.group[group], nil
}

func newPopulatedAKSMock() *mockAKSAPI {
	return &mockAKSAPI{
		group: aksMockClusters,
	}
}

var aksMockClusters = map[string][]*azure.AKSCluster{
	"group1": {
		{
			Name:           "aks-cluster1",
			GroupName:      "group1",
			TenantID:       "tenantID",
			Location:       "uswest1",
			SubscriptionID: "subID",
			Tags: map[string]string{
				"env":      "prod",
				"location": "uswest1",
			},
			Properties: azure.AKSClusterProperties{},
		},
		{
			Name:           "aks-cluster2",
			GroupName:      "group1",
			TenantID:       "tenantID",
			Location:       "uswest2",
			SubscriptionID: "subID",
			Tags: map[string]string{
				"env":      "prod",
				"location": "uswest1",
			},
			Properties: azure.AKSClusterProperties{},
		},
	},
	"group2": {
		{
			Name:           "aks-cluster3",
			GroupName:      "group1",
			TenantID:       "tenantID",
			Location:       "uswest1",
			SubscriptionID: "subID",
			Tags: map[string]string{
				"env":      "stg",
				"location": "uswest1",
			},
			Properties: azure.AKSClusterProperties{},
		},
		{
			Name:           "aks-cluster4",
			GroupName:      "group1",
			TenantID:       "tenantID",
			Location:       "uswest2",
			SubscriptionID: "subID",
			Tags: map[string]string{
				"env":      "stg",
				"location": "uswest1",
			},
			Properties: azure.AKSClusterProperties{},
		},
	},
}

type mockEKSAPI struct {
	fetchers.EKSClient
	clusters []*ekstypes.Cluster
}

func (m *mockEKSAPI) ListClusters(ctx context.Context, req *eks.ListClustersInput, _ ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	var names []string
	for _, cluster := range m.clusters {
		names = append(names, aws.ToString(cluster.Name))
	}

	// First call, no NextToken. Return first half and a NextToken value.
	if req.NextToken == nil {
		return &eks.ListClustersOutput{
			Clusters:  names[:len(names)/2],
			NextToken: aws.String("next"),
		}, nil
	}

	// Second call, we have a NextToken, return the second half.
	return &eks.ListClustersOutput{
		Clusters: names[len(names)/2:],
	}, nil
}

func (m *mockEKSAPI) DescribeCluster(_ context.Context, req *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	for _, cluster := range m.clusters {
		if aws.ToString(cluster.Name) == aws.ToString(req.Name) {
			return &eks.DescribeClusterOutput{
				Cluster: cluster,
			}, nil
		}
	}
	return nil, errors.New("cluster not found")
}

func newPopulatedEKSMock() *mockEKSAPI {
	return &mockEKSAPI{
		clusters: eksMockClusters,
	}
}

type mockFetchersClients struct {
	mocks.AWSConfigProvider
	eksClusters []*ekstypes.Cluster
}

func (m *mockFetchersClients) GetAWSEKSClient(aws.Config) fetchers.EKSClient {
	return &mockEKSAPI{
		clusters: m.eksClusters,
	}
}

func (m *mockFetchersClients) GetAWSSTSClient(aws.Config) fetchers.STSClient {
	if m.AWSConfigProvider.STSClient != nil {
		return m.AWSConfigProvider.STSClient
	}
	return &mocks.STSClient{}
}

func (m *mockFetchersClients) GetAWSSTSPresignClient(aws.Config) fetchers.STSPresignClient {
	return nil
}

var eksMockClusters = []*ekstypes.Cluster{
	{
		Name:   aws.String("eks-cluster1"),
		Arn:    aws.String("arn:aws:eks:eu-west-1:accountID:cluster/cluster1"),
		Status: ekstypes.ClusterStatusActive,
		Tags: map[string]string{
			"env":      "prod",
			"location": "eu-west-1",
		},
	},
	{
		Name:   aws.String("eks-cluster2"),
		Arn:    aws.String("arn:aws:eks:eu-west-1:accountID:cluster/cluster2"),
		Status: ekstypes.ClusterStatusActive,
		Tags: map[string]string{
			"env":      "prod",
			"location": "eu-west-1",
		},
	},

	{
		Name:   aws.String("eks-cluster3"),
		Arn:    aws.String("arn:aws:eks:eu-west-1:accountID:cluster/cluster3"),
		Status: ekstypes.ClusterStatusActive,
		Tags: map[string]string{
			"env":      "stg",
			"location": "eu-west-1",
		},
	},
	{
		Name:   aws.String("eks-cluster4"),
		Arn:    aws.String("arn:aws:eks:eu-west-1:accountID:cluster/cluster1"),
		Status: ekstypes.ClusterStatusActive,
		Tags: map[string]string{
			"env":      "stg",
			"location": "eu-west-1",
		},
	},
}

func mustConvertEKSToKubeCluster(t *testing.T, eksCluster *ekstypes.Cluster, discoveryParams rewriteDiscoveryLabelsParams) types.KubeCluster {
	cluster, err := common.NewKubeClusterFromAWSEKS(aws.ToString(eksCluster.Name), aws.ToString(eksCluster.Arn), eksCluster.Tags)
	require.NoError(t, err)
	discoveryParams.matcherType = types.AWSMatcherEKS
	rewriteCloudResource(t, cluster, discoveryParams)
	return cluster
}

func mustConvertAKSToKubeCluster(t *testing.T, azureCluster *azure.AKSCluster, discoveryParams rewriteDiscoveryLabelsParams) types.KubeCluster {
	cluster, err := common.NewKubeClusterFromAzureAKS(azureCluster)
	require.NoError(t, err)
	discoveryParams.matcherType = types.AzureMatcherKubernetes
	rewriteCloudResource(t, cluster, discoveryParams)
	return cluster
}

func modifyKubeCluster(cluster types.KubeCluster) types.KubeCluster {
	cluster.GetStaticLabels()["test"] = "test"
	return cluster
}

func sliceToSet[T comparable](slice []T) map[T]struct{} {
	set := map[T]struct{}{}
	for _, v := range slice {
		set[v] = struct{}{}
	}
	return set
}

func mustConvertKubeServiceToApp(t *testing.T, discoveryGroup, protocol string, kubeService *corev1.Service, port corev1.ServicePort) types.Application {
	port.Name = ""
	app, err := services.NewApplicationFromKubeService(*kubeService, discoveryGroup, protocol, port)
	require.NoError(t, err)
	require.Equal(t, kubeService.Annotations[types.DiscoveryPublicAddr], app.GetPublicAddr())
	if p, ok := kubeService.Annotations[types.DiscoveryPathLabel]; ok {
		components := strings.Split(p, "/")
		for i := range components {
			components[i] = url.PathEscape(components[i])
		}
		require.True(t, strings.HasSuffix(app.GetURI(), "/"+strings.Join(components, "/")), "uri: %v", app.GetURI())
	}

	app.GetStaticLabels()[types.TeleportInternalDiscoveryGroupName] = discoveryGroup
	app.GetStaticLabels()[types.OriginLabel] = types.OriginDiscoveryKubernetes
	app.GetStaticLabels()[types.DiscoveryTypeLabel] = types.KubernetesMatchersApp
	return app
}

func newPopulatedGCPMock() *mockGKEAPI {
	return &mockGKEAPI{
		clusters: gkeMockClusters,
	}
}

var gkeMockClusters = []gcp.GKECluster{
	{
		Name:   "cluster1",
		Status: containerpb.Cluster_RUNNING,
		Labels: map[string]string{
			"env":      "prod",
			"location": "central-1",
		},
		ProjectID:   "p1",
		Location:    "central-1",
		Description: "desc1",
	},
	{
		Name:   "cluster2",
		Status: containerpb.Cluster_RUNNING,
		Labels: map[string]string{
			"env":      "prod",
			"location": "central-1",
		},
		ProjectID:   "p1",
		Location:    "central-1",
		Description: "desc1",
	},
	{
		Name:   "cluster3",
		Status: containerpb.Cluster_RUNNING,
		Labels: map[string]string{
			"env":      "stg",
			"location": "central-1",
		},
		ProjectID:   "p1",
		Location:    "central-1",
		Description: "desc1",
	},
	{
		Name:   "cluster4",
		Status: containerpb.Cluster_RUNNING,
		Labels: map[string]string{
			"env":      "stg",
			"location": "central-1",
		},
		ProjectID:   "p1",
		Location:    "central-1",
		Description: "desc1",
	},
	{
		Name:   "cluster5",
		Status: containerpb.Cluster_RUNNING,
		Labels: map[string]string{
			"env":      "prod",
			"location": "central-1",
		},
		ProjectID:   "p2",
		Location:    "central-1",
		Description: "desc1",
	},
	{
		Name:   "cluster6",
		Status: containerpb.Cluster_RUNNING,
		Labels: map[string]string{
			"env":      "stg",
			"location": "central-1",
		},
		ProjectID:   "p2",
		Location:    "central-1",
		Description: "desc1",
	},
}

func mustConvertGKEToKubeCluster(t *testing.T, gkeCluster gcp.GKECluster, discoveryParams rewriteDiscoveryLabelsParams) types.KubeCluster {
	cluster, err := common.NewKubeClusterFromGCPGKE(gkeCluster)
	require.NoError(t, err)
	discoveryParams.matcherType = types.GCPMatcherKubernetes
	rewriteCloudResource(t, cluster, discoveryParams)
	return cluster
}

type mockGKEAPI struct {
	gcp.GKEClient
	clusters []gcp.GKECluster
}

func (m *mockGKEAPI) ListClusters(ctx context.Context, projectID string, location string) ([]gcp.GKECluster, error) {
	var clusters []gcp.GKECluster
	for _, cluster := range m.clusters {
		if cluster.ProjectID != projectID {
			continue
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

func TestDiscoveryDatabase(t *testing.T) {
	const (
		mainDiscoveryGroup  = "main"
		integrationName     = "my-integration"
		discoveryConfigName = "my-discovery-config"
	)
	awsRedshiftResource, awsRedshiftDB := makeRedshiftCluster(t, "aws-redshift", "us-east-1", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup})
	_, awsRedshiftDBWithIntegration := makeRedshiftCluster(t, "aws-redshift", "us-east-1", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup, integration: integrationName})
	_, awsRedshiftDBWithIntegrationAndDiscoveryConfig := makeRedshiftCluster(t, "aws-redshift", "us-east-1", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup, integration: integrationName, discoveryConfigName: discoveryConfigName})
	_, awsRedshiftDBWithDiscoveryConfig := makeRedshiftCluster(t, "aws-redshift", "us-east-1", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup, discoveryConfigName: discoveryConfigName})
	awsRDSInstance, awsRDSDB := makeRDSInstance(t, "aws-rds", "us-west-1", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup})
	azRedisResource, azRedisDB := makeAzureRedisServer(t, "az-redis", "sub1", "group1", "East US", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup})
	_, azRedisDBWithDiscoveryConfig := makeAzureRedisServer(t, "az-redis", "sub1", "group1", "East US", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup, discoveryConfigName: discoveryConfigName})

	role := types.AssumeRole{RoleARN: "arn:aws:iam::123456789012:role/test-role", ExternalID: "test123"}
	awsRDSDBWithRole := awsRDSDB.Copy()
	awsRDSDBWithRole.SetAWSAssumeRole("arn:aws:iam::123456789012:role/test-role")
	awsRDSDBWithRole.SetAWSExternalID("test123")

	awsRDSDBWithIntegration := awsRDSDB.Copy()
	rewriteCloudResource(t, awsRDSDBWithIntegration, rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup, integration: integrationName, discoveryConfigName: discoveryConfigName})

	eksAWSResource, _ := makeEKSCluster(t, "aws-eks", "us-east-1", rewriteDiscoveryLabelsParams{discoveryGroup: mainDiscoveryGroup, integration: integrationName, discoveryConfigName: discoveryConfigName})

	matcherForDiscoveryConfigFn := func(t *testing.T, discoveryGroup string, m Matchers) *discoveryconfig.DiscoveryConfig {
		dc, err := discoveryconfig.NewDiscoveryConfig(
			header.Metadata{Name: discoveryConfigName},
			discoveryconfig.Spec{
				DiscoveryGroup: discoveryGroup,
				AWS:            m.AWS,
				Azure:          m.Azure,
				GCP:            m.GCP,
				Kube:           m.Kubernetes,
			},
		)

		require.NoError(t, err)
		return dc
	}

	testCloudClients := &cloud.TestCloudClients{
		AzureRedis: azure.NewRedisClientByAPI(&azure.ARMRedisMock{
			Servers: []*armredis.ResourceInfo{azRedisResource},
		}),
		AzureRedisEnterprise: azure.NewRedisEnterpriseClientByAPI(
			&azure.ARMRedisEnterpriseClusterMock{},
			&azure.ARMRedisEnterpriseDatabaseMock{},
		),
	}

	tcs := []struct {
		name                                   string
		existingDatabases                      []types.Database
		integrationsOnlyCredentials            bool
		awsMatchers                            []types.AWSMatcher
		azureMatchers                          []types.AzureMatcher
		expectDatabases                        []types.Database
		discoveryConfigs                       func(*testing.T) []*discoveryconfig.DiscoveryConfig
		discoveryConfigStatusCheck             func(*testing.T, discoveryconfig.Status)
		discoveryConfigStatusExpectedResources int
		userTasksCheck                         func(*testing.T, []*usertasksv1.UserTask)
		wantEvents                             int
	}{
		{
			name: "discover AWS database",
			awsMatchers: []types.AWSMatcher{{
				Types:   []string{types.AWSMatcherRedshift},
				Tags:    map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
				Regions: []string{"us-east-1"},
			}},
			expectDatabases: []types.Database{awsRedshiftDB},
			wantEvents:      1,
		},
		{
			name: "discover AWS database with assumed role",
			awsMatchers: []types.AWSMatcher{{
				Types:      []string{types.AWSMatcherRDS},
				Tags:       map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
				Regions:    []string{"us-west-1"},
				AssumeRole: &role,
			}},
			expectDatabases: []types.Database{awsRDSDBWithRole},
			wantEvents:      1,
		},
		{
			name: "discover Azure database",
			azureMatchers: []types.AzureMatcher{{
				Types:          []string{types.AzureMatcherRedis},
				ResourceTags:   map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
				Regions:        []string{types.Wildcard},
				ResourceGroups: []string{types.Wildcard},
				Subscriptions:  []string{"sub1"},
			}},
			expectDatabases: []types.Database{azRedisDB},
			wantEvents:      1,
		},
		{
			name: "update existing database",
			existingDatabases: []types.Database{
				mustNewDatabase(t, types.Metadata{
					Name:        awsRedshiftDB.GetName(),
					Description: "should be updated",
					Labels:      map[string]string{types.OriginLabel: types.OriginCloud, types.TeleportInternalDiscoveryGroupName: mainDiscoveryGroup},
				}, types.DatabaseSpecV3{
					Protocol: "redis",
					URI:      "should.be.updated.com:12345",
					AWS: types.AWS{
						Redshift: types.Redshift{
							ClusterID: "aws-redshift",
						},
					},
				}),
			},
			awsMatchers: []types.AWSMatcher{{
				Types:   []string{types.AWSMatcherRedshift},
				Tags:    map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
				Regions: []string{"us-east-1"},
			}},
			expectDatabases: []types.Database{awsRedshiftDB},
		},
		{
			name: "update existing database with assumed role",
			existingDatabases: []types.Database{
				mustNewDatabase(t, types.Metadata{
					Name:        awsRDSDBWithRole.GetName(),
					Description: "should be updated",
					Labels:      map[string]string{types.OriginLabel: types.OriginCloud, types.TeleportInternalDiscoveryGroupName: mainDiscoveryGroup},
				}, types.DatabaseSpecV3{
					Protocol: "postgres",
					URI:      "should.be.updated.com:12345",
				}),
			},
			awsMatchers: []types.AWSMatcher{{
				Types:      []string{types.AWSMatcherRDS},
				Tags:       map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
				Regions:    []string{"us-west-1"},
				AssumeRole: &role,
			}},
			expectDatabases: []types.Database{awsRDSDBWithRole},
		},
		{
			name: "delete existing database",
			existingDatabases: []types.Database{
				mustNewDatabase(t, types.Metadata{
					Name:        awsRedshiftDB.GetName(),
					Description: "should not be deleted",
					Labels:      map[string]string{types.OriginLabel: types.OriginCloud},
				}, types.DatabaseSpecV3{
					Protocol: "redis",
					URI:      "should.not.be.deleted.com:12345",
				}),
			},
			awsMatchers: []types.AWSMatcher{{
				Types:   []string{types.AWSMatcherRedshift},
				Tags:    map[string]utils.Strings{"do-not-match": {"do-not-match"}},
				Regions: []string{"us-east-1"},
			}},
			expectDatabases: []types.Database{
				mustNewDatabase(t, types.Metadata{
					Name:        awsRedshiftDB.GetName(),
					Description: "should not be deleted",
					Labels:      map[string]string{types.OriginLabel: types.OriginCloud},
				}, types.DatabaseSpecV3{
					Protocol: "redis",
					URI:      "should.not.be.deleted.com:12345",
				}),
			},
		},
		{
			name: "skip self-hosted database",
			existingDatabases: []types.Database{
				mustNewDatabase(t, types.Metadata{
					Name:        "self-hosted",
					Description: "should be ignored (not deleted)",
					Labels:      map[string]string{types.OriginLabel: types.OriginConfigFile},
				}, types.DatabaseSpecV3{
					Protocol: "mysql",
					URI:      "localhost:12345",
				}),
			},
			awsMatchers: []types.AWSMatcher{{
				Types:   []string{types.AWSMatcherRedshift},
				Tags:    map[string]utils.Strings{"do-not-match": {"do-not-match"}},
				Regions: []string{"us-east-1"},
			}},
			expectDatabases: []types.Database{
				mustNewDatabase(t, types.Metadata{
					Name:        "self-hosted",
					Description: "should be ignored (not deleted)",
					Labels:      map[string]string{types.OriginLabel: types.OriginConfigFile},
				}, types.DatabaseSpecV3{
					Protocol: "mysql",
					URI:      "localhost:12345",
				}),
			},
		},
		{
			name:            "discover Azure database using dynamic matchers",
			expectDatabases: []types.Database{azRedisDBWithDiscoveryConfig},
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					Azure: []types.AzureMatcher{{
						Types:          []string{types.AzureMatcherRedis},
						ResourceTags:   map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions:        []string{types.Wildcard},
						ResourceGroups: []string{types.Wildcard},
						Subscriptions:  []string{"sub1"},
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			wantEvents: 1,
		},
		{
			name:            "discover AWS database using dynamic matchers",
			expectDatabases: []types.Database{awsRedshiftDBWithDiscoveryConfig},
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					AWS: []types.AWSMatcher{{
						Types:   []string{types.AWSMatcherRedshift},
						Tags:    map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions: []string{"us-east-1"},
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			wantEvents: 1,
		},
		{
			name:                        "running in integrations-only-mode with a matcher without an integration, must discard the dynamic matcher and find 0 databases",
			integrationsOnlyCredentials: true,
			expectDatabases:             []types.Database{},
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					AWS: []types.AWSMatcher{{
						Types:   []string{types.AWSMatcherRedshift},
						Tags:    map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions: []string{"us-east-1"},
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			wantEvents: 0,
		},
		{
			name:            "running in integrations-only-mode with a dynamic matcher with an integration, must find 1 database",
			expectDatabases: []types.Database{awsRedshiftDBWithIntegrationAndDiscoveryConfig},
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					AWS: []types.AWSMatcher{{
						Types:       []string{types.AWSMatcherRedshift},
						Tags:        map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions:     []string{"us-east-1"},
						Integration: integrationName,
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			wantEvents: 1,
			discoveryConfigStatusCheck: func(t *testing.T, s discoveryconfig.Status) {
				require.Equal(t, uint64(1), s.IntegrationDiscoveredResources[integrationName].AwsRds.Enrolled)
				require.Equal(t, uint64(1), s.IntegrationDiscoveredResources[integrationName].AwsRds.Found)
				require.Zero(t, s.IntegrationDiscoveredResources[integrationName].AwsRds.Failed)
			},
			discoveryConfigStatusExpectedResources: 1,
		},
		{
			name:                        "running in integrations-only-mode with a matcher without an integration, must find 1 database",
			integrationsOnlyCredentials: true,
			awsMatchers: []types.AWSMatcher{{
				Types:       []string{types.AWSMatcherRedshift},
				Tags:        map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
				Regions:     []string{"us-east-1"},
				Integration: integrationName,
			}},
			expectDatabases: []types.Database{awsRedshiftDBWithIntegration},
			wantEvents:      1,
		},
		{
			name: "running in integrations-only-mode with a dynamic matcher with an integration, must find 1 eks cluster",
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					AWS: []types.AWSMatcher{{
						Types:       []string{types.AWSMatcherEKS},
						Tags:        map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions:     []string{"us-east-1"},
						Integration: integrationName,
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			expectDatabases: []types.Database{},
			wantEvents:      0,
			discoveryConfigStatusCheck: func(t *testing.T, s discoveryconfig.Status) {
				require.Equal(t, uint64(1), s.IntegrationDiscoveredResources[integrationName].AwsEks.Found)
				require.Zero(t, s.IntegrationDiscoveredResources[integrationName].AwsEks.Enrolled)
			},
			discoveryConfigStatusExpectedResources: 1,
		},
		{
			name: "discovery config status must be updated even when there are no resources",
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					AWS: []types.AWSMatcher{{
						// ElastiCache and MemoryDB fake clients return no resources.
						Types:       []string{types.AWSMatcherElastiCache, types.AWSMatcherMemoryDB},
						Tags:        map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions:     []string{"us-east-1"},
						Integration: integrationName,
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			expectDatabases: []types.Database{},
			wantEvents:      0,
			discoveryConfigStatusCheck: func(t *testing.T, s discoveryconfig.Status) {
				require.Equal(t, "DISCOVERY_CONFIG_STATE_SYNCING", s.State)
			},
			discoveryConfigStatusExpectedResources: 0,
		},
		{
			name: "discover-rds user task must be created when database is not configured to allow IAM DB Authentication",
			discoveryConfigs: func(t *testing.T) []*discoveryconfig.DiscoveryConfig {
				dc1 := matcherForDiscoveryConfigFn(t, mainDiscoveryGroup, Matchers{
					AWS: []types.AWSMatcher{{
						Types:       []string{types.AWSMatcherRDS},
						Tags:        map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
						Regions:     []string{"us-west-1"},
						Integration: integrationName,
					}},
				})
				return []*discoveryconfig.DiscoveryConfig{dc1}
			},
			expectDatabases: []types.Database{awsRDSDBWithIntegration},
			wantEvents:      1,
			userTasksCheck: func(t *testing.T, uts []*usertasksv1.UserTask) {
				require.Len(t, uts, 1)
				gotUserTask := uts[0]
				require.Equal(t, "3ae76664-b54d-5b74-b59a-bd7bff3be053", gotUserTask.GetMetadata().GetName())
				require.Equal(t, "OPEN", gotUserTask.GetSpec().GetState())
				require.Equal(t, "discover-rds", gotUserTask.GetSpec().GetTaskType())
				require.Equal(t, "rds-iam-auth-disabled", gotUserTask.GetSpec().GetIssueType())
				require.Equal(t, "my-integration", gotUserTask.GetSpec().GetIntegration())

				require.NotNil(t, gotUserTask.GetSpec().GetDiscoverRds())
				require.Equal(t, "123456789012", gotUserTask.GetSpec().GetDiscoverRds().GetAccountId())
				require.Equal(t, "us-west-1", gotUserTask.GetSpec().GetDiscoverRds().GetRegion())

				require.Contains(t, gotUserTask.GetSpec().GetDiscoverRds().GetDatabases(), "aws-rds")
				gotDatabase := gotUserTask.GetSpec().GetDiscoverRds().GetDatabases()["aws-rds"]
				require.Equal(t, "my-discovery-config", gotDatabase.DiscoveryConfig)
				require.Equal(t, "main", gotDatabase.DiscoveryGroup)
				require.Equal(t, "postgres", gotDatabase.Engine)
				require.Equal(t, "aws-rds", gotDatabase.Name)
				require.False(t, gotDatabase.IsCluster)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeClock := clockwork.NewFakeClock()

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			// Create and start test auth server.
			testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
				Dir: t.TempDir(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

			tlsServer, err := testAuthServer.NewTestTLSServer()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

			awsOIDCIntegration, err := types.NewIntegrationAWSOIDC(types.Metadata{
				Name: integrationName,
			}, &types.AWSOIDCIntegrationSpecV1{
				RoleARN: "arn:aws:iam::123456789012:role/teleport",
			})
			require.NoError(t, err)

			testAuthServer.AuthServer.IntegrationsTokenGenerator = &mockIntegrationsTokenGenerator{
				proxies: nil,
				integrations: map[string]types.Integration{
					awsOIDCIntegration.GetName(): awsOIDCIntegration,
				},
			}

			_, err = tlsServer.Auth().CreateIntegration(ctx, awsOIDCIntegration)
			require.NoError(t, err)

			// Auth client for discovery service.
			identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
			authClient, err := tlsServer.NewClient(identity)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, authClient.Close()) })

			for _, database := range tc.existingDatabases {
				err := tlsServer.Auth().CreateDatabase(ctx, database)
				require.NoError(t, err)
			}

			integrationOnlyCredential := tc.integrationsOnlyCredentials
			waitForReconcile := make(chan struct{})
			reporter := &mockUsageReporter{}
			tlsServer.Auth().SetUsageReporter(reporter)
			accessPoint := getDiscoveryAccessPoint(tlsServer.Auth(), authClient)
			fakeConfigProvider := &mocks.AWSConfigProvider{
				OIDCIntegrationClient: accessPoint,
			}
			dbFetcherFactory, err := db.NewAWSFetcherFactory(db.AWSFetcherFactoryConfig{
				AWSConfigProvider: fakeConfigProvider,
				AWSClients: fakeAWSClients{
					ecClient:  &mocks.ElastiCacheClient{},
					mdbClient: &mocks.MemoryDBClient{},
					rdsClient: &mocks.RDSClient{
						DBInstances: []rdstypes.DBInstance{*awsRDSInstance},
						DBEngineVersions: []rdstypes.DBEngineVersion{
							{Engine: aws.String(services.RDSEnginePostgres)},
						},
					},
					redshiftClient: &mocks.RedshiftClient{
						Clusters: []redshifttypes.Cluster{*awsRedshiftResource},
					},
				},
			})
			require.NoError(t, err)

			srv, err := New(
				authz.ContextWithUser(ctx, identity.I),
				&Config{
					IntegrationOnlyCredentials: integrationOnlyCredential,
					AWSFetchersClients: &mockFetchersClients{
						AWSConfigProvider: *fakeConfigProvider,
						eksClusters:       []*ekstypes.Cluster{eksAWSResource},
					},
					CloudClients:              testCloudClients,
					ClusterFeatures:           func() proto.Features { return proto.Features{} },
					KubernetesClient:          fake.NewSimpleClientset(),
					AccessPoint:               getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
					AWSDatabaseFetcherFactory: dbFetcherFactory,
					AWSConfigProvider:         fakeConfigProvider,
					Matchers: Matchers{
						AWS:   tc.awsMatchers,
						Azure: tc.azureMatchers,
					},
					Emitter: authClient,
					onDatabaseReconcile: func() {
						waitForReconcile <- struct{}{}
					},
					DiscoveryGroup: mainDiscoveryGroup,
					clock:          fakeClock,
				})

			require.NoError(t, err)

			// Add Dynamic Matchers and wait for reconcile again
			if tc.discoveryConfigs != nil {
				for _, dc := range tc.discoveryConfigs(t) {
					_, err := tlsServer.Auth().DiscoveryConfigs.CreateDiscoveryConfig(ctx, dc)
					require.NoError(t, err)
				}

				// Wait for the DiscoveryConfig to be added to the dynamic matchers
				require.Eventually(t, func() bool {
					srv.muDynamicDatabaseFetchers.RLock()
					defer srv.muDynamicDatabaseFetchers.RUnlock()
					return len(srv.dynamicDatabaseFetchers) > 0
				}, 1*time.Second, 100*time.Millisecond)
			}

			t.Cleanup(srv.Stop)
			go srv.Start()

			select {
			case <-waitForReconcile:
				// Use tlsServer.Auth() instead of authClient to compare
				// databases stored in auth. authClient was created with
				// types.RoleDiscovery and it does not have permissions to
				// access non-cloud databases.
				actualDatabases, err := tlsServer.Auth().GetDatabases(ctx)
				require.NoError(t, err)
				require.Empty(t, cmp.Diff(tc.expectDatabases, actualDatabases,
					cmpopts.IgnoreFields(types.Metadata{}, "Revision"),
					cmpopts.IgnoreFields(types.DatabaseStatusV3{}, "CACert"),
				))
			case <-time.After(time.Second):
				require.FailNow(t, "Didn't receive reconcile event after 1s")
			}

			if tc.wantEvents > 0 {
				require.Eventually(t, func() bool {
					return reporter.ResourceCreateEventCount() == tc.wantEvents
				}, time.Second, 100*time.Millisecond)
			} else {
				require.Never(t, func() bool {
					return reporter.ResourceCreateEventCount() != 0
				}, time.Second, 100*time.Millisecond)
			}

			if tc.discoveryConfigStatusCheck != nil {
				require.Eventually(t, func() bool {
					fakeClock.Advance(srv.PollInterval * 2)
					dc, err := tlsServer.Auth().GetDiscoveryConfig(ctx, discoveryConfigName)
					require.NoError(t, err)
					if tc.discoveryConfigStatusExpectedResources != int(dc.Status.DiscoveredResources) {
						return false
					}

					tc.discoveryConfigStatusCheck(t, dc.Status)
					return true
				}, time.Second, 100*time.Millisecond)

			}
			if tc.userTasksCheck != nil {
				var userTasks []*usertasksv1.UserTask
				var nextPage string
				for {
					filters := &usertasksv1.ListUserTasksFilters{
						Integration: integrationName,
					}
					userTasksResp, nextPageResp, err := tlsServer.Auth().ListUserTasks(ctx, 0, nextPage, filters)
					require.NoError(t, err)

					userTasks = append(userTasks, userTasksResp...)

					if nextPageResp == "" {
						break
					}
					nextPage = nextPageResp
				}
				tc.userTasksCheck(t, userTasks)
			}
		})
	}
}

func TestDiscoveryDatabaseRemovingDiscoveryConfigs(t *testing.T) {
	const mainDiscoveryGroup = "main"

	clock := clockwork.NewFakeClock()
	dc1Name := uuid.NewString()
	dc2Name := uuid.NewString()

	awsRDSInstance, awsRDSDB := makeRDSInstance(t, "aws-rds", "us-west-1", rewriteDiscoveryLabelsParams{discoveryConfigName: dc2Name, discoveryGroup: mainDiscoveryGroup})

	fakeConfigProvider := &mocks.AWSConfigProvider{
		STSClient: &mocks.STSClient{},
	}
	dbFetcherFactory, err := db.NewAWSFetcherFactory(db.AWSFetcherFactoryConfig{
		AWSConfigProvider: fakeConfigProvider,
		AWSClients: fakeAWSClients{
			rdsClient: &mocks.RDSClient{
				DBInstances: []rdstypes.DBInstance{*awsRDSInstance},
				DBEngineVersions: []rdstypes.DBEngineVersion{
					{Engine: aws.String(services.RDSEnginePostgres)},
				},
			},
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Create and start test auth server.
	testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
		Dir: t.TempDir(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

	tlsServer, err := testAuthServer.NewTestTLSServer()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

	// Auth client for discovery service.
	identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
	authClient, err := tlsServer.NewClient(identity)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authClient.Close()) })

	waitForReconcileTimeout := 5 * time.Second
	reporter := &mockUsageReporter{}
	tlsServer.Auth().SetUsageReporter(reporter)
	srv, err := New(
		authz.ContextWithUser(ctx, identity.I),
		&Config{
			AWSConfigProvider:         fakeConfigProvider,
			AWSDatabaseFetcherFactory: dbFetcherFactory,
			ClusterFeatures:           func() proto.Features { return proto.Features{} },
			KubernetesClient:          fake.NewSimpleClientset(),
			AccessPoint:               getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
			Matchers:                  Matchers{},
			Emitter:                   authClient,
			DiscoveryGroup:            mainDiscoveryGroup,
			clock:                     clock,
		})

	require.NoError(t, err)

	t.Cleanup(srv.Stop)
	go srv.Start()

	// First Reconcile should not have any databases
	actualDatabases, err := tlsServer.Auth().GetDatabases(ctx)
	require.NoError(t, err)
	require.Empty(t, actualDatabases)

	require.Zero(t, reporter.DiscoveryFetchEventCount(), "a fetch event was emitted but there is no fetchers actually being called")

	// Adding a Dynamic Matcher for a different Discovery Group, should not bring any new resources.
	t.Run("DiscoveryGroup does not match: matcher is not loaded", func(t *testing.T) {
		// Create a Dynamic matcher
		dc1, err := discoveryconfig.NewDiscoveryConfig(
			header.Metadata{Name: dc1Name},
			discoveryconfig.Spec{
				DiscoveryGroup: "another-discovery-group",
				AWS: []types.AWSMatcher{{
					Types:   []string{types.AWSMatcherRDS},
					Tags:    map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
					Regions: []string{"us-west-1"},
				}},
			},
		)
		require.NoError(t, err)

		_, err = tlsServer.Auth().DiscoveryConfigs.CreateDiscoveryConfig(ctx, dc1)
		require.NoError(t, err)

		actualDatabases, err := tlsServer.Auth().GetDatabases(ctx)
		require.NoError(t, err)
		require.Empty(t, actualDatabases)

		require.Zero(t, reporter.DiscoveryFetchEventCount(), "a fetch event was emitted but there is no fetchers actually being called")
	})

	t.Run("New DiscoveryConfig with valid Group", func(t *testing.T) {
		// Create a Dynamic matcher
		dc2, err := discoveryconfig.NewDiscoveryConfig(
			header.Metadata{Name: dc2Name},
			discoveryconfig.Spec{
				DiscoveryGroup: mainDiscoveryGroup,
				AWS: []types.AWSMatcher{{
					Types:   []string{types.AWSMatcherRDS},
					Tags:    map[string]utils.Strings{types.Wildcard: {types.Wildcard}},
					Regions: []string{"us-west-1"},
				}},
			},
		)
		require.NoError(t, err)

		require.Zero(t, reporter.DiscoveryFetchEventCount())
		_, err = tlsServer.Auth().DiscoveryConfigs.CreateDiscoveryConfig(ctx, dc2)
		require.NoError(t, err)

		// Check for new resource in reconciler
		expectDatabases := []types.Database{awsRDSDB}
		require.EventuallyWithT(t, func(t *assert.CollectT) {
			actualDatabases, err := tlsServer.Auth().GetDatabases(ctx)
			if !assert.NoError(t, err) {
				return
			}
			assert.Empty(t, cmp.Diff(expectDatabases, actualDatabases,
				cmpopts.IgnoreFields(types.Metadata{}, "Revision"),
				cmpopts.IgnoreFields(types.DatabaseStatusV3{}, "CACert"),
			))
		}, waitForReconcileTimeout, 100*time.Millisecond)

		currentEmittedEvents := reporter.DiscoveryFetchEventCount()
		require.GreaterOrEqual(t, currentEmittedEvents, 1)

		// Advance clock to trigger a poll.
		clock.Advance(5 * time.Minute)
		// Wait for the cycle to complete
		// A new DiscoveryFetch event must have been emitted.
		expectedEmittedEvents := currentEmittedEvents + 1
		require.EventuallyWithT(t, func(t *assert.CollectT) {
			assert.GreaterOrEqual(t, reporter.DiscoveryFetchEventCount(), expectedEmittedEvents)
		}, waitForReconcileTimeout, 100*time.Millisecond)

		t.Run("removing the DiscoveryConfig: fetcher is removed and database is removed", func(t *testing.T) {
			// Remove DiscoveryConfig
			err = tlsServer.Auth().DiscoveryConfigs.DeleteDiscoveryConfig(ctx, dc2.GetName())
			require.NoError(t, err)

			currentEmittedEvents := reporter.DiscoveryFetchEventCount()
			// Existing databases must be removed.
			require.EventuallyWithT(t, func(t *assert.CollectT) {
				actualDatabases, err := tlsServer.Auth().GetDatabases(ctx)
				if !assert.NoError(t, err) {
					return
				}
				assert.Empty(t, actualDatabases)
			}, waitForReconcileTimeout, 100*time.Millisecond)

			// Given that no Fetch was issued, the counter should not increment.
			require.Equal(t, reporter.DiscoveryFetchEventCount(), currentEmittedEvents)
		})
	})
}

func makeEKSCluster(t *testing.T, name, region string, discoveryParams rewriteDiscoveryLabelsParams) (*ekstypes.Cluster, types.KubeCluster) {
	t.Helper()
	eksAWSCluster := &ekstypes.Cluster{
		Name:   aws.String(name),
		Arn:    aws.String(fmt.Sprintf("arn:aws:eks:%s:123456789012:cluster/%s", region, name)),
		Status: ekstypes.ClusterStatusActive,
		Tags: map[string]string{
			"env": "prod",
		},
	}
	actual, err := common.NewKubeClusterFromAWSEKS(aws.ToString(eksAWSCluster.Name), aws.ToString(eksAWSCluster.Arn), eksAWSCluster.Tags)
	require.NoError(t, err)
	discoveryParams.matcherType = types.AWSMatcherEKS
	rewriteCloudResource(t, actual, discoveryParams)
	return eksAWSCluster, actual
}

func makeRDSInstance(t *testing.T, name, region string, discoveryParams rewriteDiscoveryLabelsParams) (*rdstypes.DBInstance, types.Database) {
	instance := &rdstypes.DBInstance{
		DBInstanceArn:        aws.String(fmt.Sprintf("arn:aws:rds:%v:123456789012:db:%v", region, name)),
		DBInstanceIdentifier: aws.String(name),
		DbiResourceId:        aws.String(uuid.New().String()),
		Engine:               aws.String(services.RDSEnginePostgres),
		DBInstanceStatus:     aws.String("available"),
		Endpoint: &rdstypes.Endpoint{
			Address: aws.String("localhost"),
			Port:    aws.Int32(5432),
		},
	}
	database, err := common.NewDatabaseFromRDSInstance(instance)
	require.NoError(t, err)
	discoveryParams.matcherType = types.AWSMatcherRDS
	rewriteCloudResource(t, database, discoveryParams)
	return instance, database
}

func makeRedshiftCluster(t *testing.T, name, region string, discoveryParams rewriteDiscoveryLabelsParams) (*redshifttypes.Cluster, types.Database) {
	t.Helper()
	cluster := &redshifttypes.Cluster{
		ClusterIdentifier:   aws.String(name),
		ClusterNamespaceArn: aws.String(fmt.Sprintf("arn:aws:redshift:%s:123456789012:namespace:%s", region, name)),
		ClusterStatus:       aws.String("available"),
		Endpoint: &redshifttypes.Endpoint{
			Address: aws.String("localhost"),
			Port:    aws.Int32(5439),
		},
	}

	database, err := common.NewDatabaseFromRedshiftCluster(cluster)
	require.NoError(t, err)
	discoveryParams.matcherType = types.AWSMatcherRedshift
	rewriteCloudResource(t, database, discoveryParams)
	return cluster, database
}

func makeAzureRedisServer(t *testing.T, name, subscription, group, region string, discoveryParams rewriteDiscoveryLabelsParams) (*armredis.ResourceInfo, types.Database) {
	t.Helper()
	resourceInfo := &armredis.ResourceInfo{
		Name:     to.Ptr(name),
		ID:       to.Ptr(fmt.Sprintf("/subscriptions/%v/resourceGroups/%v/providers/Microsoft.Cache/Redis/%v", subscription, group, name)),
		Location: to.Ptr(region),
		Properties: &armredis.Properties{
			HostName:          to.Ptr(fmt.Sprintf("%v.redis.cache.windows.net", name)),
			SSLPort:           to.Ptr(int32(6380)),
			ProvisioningState: to.Ptr(armredis.ProvisioningStateSucceeded),
		},
	}

	database, err := common.NewDatabaseFromAzureRedis(resourceInfo)
	require.NoError(t, err)
	discoveryParams.matcherType = types.AzureMatcherRedis
	rewriteCloudResource(t, database, discoveryParams)
	return resourceInfo, database
}

func mustNewDatabase(t *testing.T, meta types.Metadata, spec types.DatabaseSpecV3) types.Database {
	t.Helper()
	database, err := types.NewDatabaseV3(meta, spec)
	require.NoError(t, err)
	return database
}

type mockAzureRunCommandClient struct{}

func (m *mockAzureRunCommandClient) Run(_ context.Context, _ azure.RunCommandRequest) error {
	return nil
}

type mockAzureClient struct {
	vms []*armcompute.VirtualMachine
}

func (m *mockAzureClient) Get(_ context.Context, _ string) (*azure.VirtualMachine, error) {
	return nil, nil
}

func (m *mockAzureClient) GetByVMID(_ context.Context, _ string) (*azure.VirtualMachine, error) {
	return nil, nil
}

func (m *mockAzureClient) ListVirtualMachines(_ context.Context, _ string) ([]*armcompute.VirtualMachine, error) {
	return m.vms, nil
}

type mockAzureInstaller struct {
	mu                 sync.Mutex
	installedInstances map[string]struct{}
}

func (m *mockAzureInstaller) Run(_ context.Context, req server.AzureRunRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range req.Instances {
		m.installedInstances[*inst.Name] = struct{}{}
	}
	return nil
}

func (m *mockAzureInstaller) GetInstalledInstances() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.installedInstances))
	for k := range m.installedInstances {
		keys = append(keys, k)
	}
	return keys
}

func TestAzureVMDiscovery(t *testing.T) {
	t.Parallel()

	defaultDiscoveryGroup := "dc001"

	vmMatcherFn := func() Matchers {
		return Matchers{
			Azure: []types.AzureMatcher{{
				Types:          []string{"vm"},
				Subscriptions:  []string{"testsub"},
				ResourceGroups: []string{"testrg"},
				Regions:        []string{"westcentralus"},
				ResourceTags:   types.Labels{"teleport": {"yes"}},
			}},
		}
	}

	vmMatcher := vmMatcherFn()
	defaultDiscoveryConfig, err := discoveryconfig.NewDiscoveryConfig(
		header.Metadata{Name: uuid.NewString()},
		discoveryconfig.Spec{
			DiscoveryGroup: defaultDiscoveryGroup,
			AWS:            vmMatcher.AWS,
			Azure:          vmMatcher.Azure,
			GCP:            vmMatcher.GCP,
			Kube:           vmMatcher.Kubernetes,
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name                   string
		presentVMs             []types.Server
		foundAzureVMs          []*armcompute.VirtualMachine
		discoveryConfig        *discoveryconfig.DiscoveryConfig
		staticMatchers         Matchers
		wantInstalledInstances []string
	}{
		{
			name:       "no nodes present, 1 found",
			presentVMs: []types.Server{},
			foundAzureVMs: []*armcompute.VirtualMachine{
				{
					ID: aws.String((&arm.ResourceID{
						SubscriptionID:    "testsub",
						ResourceGroupName: "rg",
						Name:              "testvm",
					}).String()),
					Name:     aws.String("testvm"),
					Location: aws.String("westcentralus"),
					Tags: map[string]*string{
						"teleport": aws.String("yes"),
					},
					Properties: &armcompute.VirtualMachineProperties{
						VMID: aws.String("test-vmid"),
					},
				},
			},
			staticMatchers:         vmMatcherFn(),
			wantInstalledInstances: []string{"testvm"},
		},
		{
			name: "nodes present, instance filtered",
			presentVMs: []types.Server{
				&types.ServerV2{
					Kind: types.KindNode,
					Metadata: types.Metadata{
						Name: "name",
						Labels: map[string]string{
							"teleport.internal/subscription-id": "testsub",
							"teleport.internal/vm-id":           "test-vmid",
						},
						Namespace: defaults.Namespace,
					},
				},
			},
			staticMatchers: vmMatcherFn(),
			foundAzureVMs: []*armcompute.VirtualMachine{
				{
					ID: aws.String((&arm.ResourceID{
						SubscriptionID:    "testsub",
						ResourceGroupName: "rg",
						Name:              "testvm",
					}).String()),
					Location: aws.String("westcentralus"),
					Tags: map[string]*string{
						"teleport": aws.String("yes"),
					},
					Properties: &armcompute.VirtualMachineProperties{
						VMID: aws.String("test-vmid"),
					},
				},
			},
		},
		{
			name: "nodes present, instance not filtered",
			presentVMs: []types.Server{
				&types.ServerV2{
					Kind: types.KindNode,
					Metadata: types.Metadata{
						Name: "name",
						Labels: map[string]string{
							"teleport.internal/subscription-id": "testsub",
							"teleport.internal/vm-id":           "alternate-vmid",
						},
						Namespace: defaults.Namespace,
					},
				},
			},
			staticMatchers: vmMatcherFn(),
			foundAzureVMs: []*armcompute.VirtualMachine{
				{
					ID: aws.String((&arm.ResourceID{
						SubscriptionID:    "testsub",
						ResourceGroupName: "rg",
						Name:              "testvm",
					}).String()),
					Name:     aws.String("testvm"),
					Location: aws.String("westcentralus"),
					Tags: map[string]*string{
						"teleport": aws.String("yes"),
					},
					Properties: &armcompute.VirtualMachineProperties{
						VMID: aws.String("test-vmid"),
					},
				},
			},
			wantInstalledInstances: []string{"testvm"},
		},
		{
			name:       "no nodes present, 1 found using dynamic matchers",
			presentVMs: []types.Server{},
			foundAzureVMs: []*armcompute.VirtualMachine{
				{
					ID: aws.String((&arm.ResourceID{
						SubscriptionID:    "testsub",
						ResourceGroupName: "rg",
						Name:              "testvm",
					}).String()),
					Name:     aws.String("testvm"),
					Location: aws.String("westcentralus"),
					Tags: map[string]*string{
						"teleport": aws.String("yes"),
					},
					Properties: &armcompute.VirtualMachineProperties{
						VMID: aws.String("test-vmid"),
					},
				},
			},
			discoveryConfig:        defaultDiscoveryConfig,
			staticMatchers:         Matchers{},
			wantInstalledInstances: []string{"testvm"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testCloudClients := &cloud.TestCloudClients{
				AzureVirtualMachines: &mockAzureClient{
					vms: tc.foundAzureVMs,
				},
				AzureRunCommand: &mockAzureRunCommandClient{},
			}

			ctx := context.Background()
			testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
				Dir: t.TempDir(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

			tlsServer, err := testAuthServer.NewTestTLSServer()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

			// Auth client for discovery service.
			identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
			authClient, err := tlsServer.NewClient(identity)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, authClient.Close()) })

			for _, instance := range tc.presentVMs {
				_, err := tlsServer.Auth().UpsertNode(ctx, instance)
				require.NoError(t, err)
			}

			logger := logtest.NewLogger()

			emitter := &mockEmitter{}
			reporter := &mockUsageReporter{}
			installer := &mockAzureInstaller{
				installedInstances: make(map[string]struct{}),
			}
			tlsServer.Auth().SetUsageReporter(reporter)
			server, err := New(authz.ContextWithUser(context.Background(), identity.I), &Config{
				CloudClients:     testCloudClients,
				ClusterFeatures:  func() proto.Features { return proto.Features{} },
				KubernetesClient: fake.NewSimpleClientset(),
				AccessPoint:      getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
				Matchers:         tc.staticMatchers,
				Emitter:          emitter,
				Log:              logger,
				DiscoveryGroup:   defaultDiscoveryGroup,
			})

			require.NoError(t, err)
			server.azureInstaller = installer
			emitter.server = server
			emitter.t = t

			if tc.discoveryConfig != nil {
				_, err := tlsServer.Auth().DiscoveryConfigs.CreateDiscoveryConfig(ctx, tc.discoveryConfig)
				require.NoError(t, err)

				// Wait for the DiscoveryConfig to be added to the dynamic matchers
				require.Eventually(t, func() bool {
					server.muDynamicServerAzureFetchers.RLock()
					defer server.muDynamicServerAzureFetchers.RUnlock()
					return len(server.dynamicServerAzureFetchers) > 0
				}, 1*time.Second, 100*time.Millisecond)
			}

			go server.Start()
			t.Cleanup(server.Stop)

			if len(tc.wantInstalledInstances) > 0 {
				require.Eventually(t, func() bool {
					instances := installer.GetInstalledInstances()
					slices.Sort(instances)
					return slices.Equal(tc.wantInstalledInstances, instances) && len(tc.wantInstalledInstances) == reporter.ResourceCreateEventCount()
				}, 500*time.Millisecond, 50*time.Millisecond)
			} else {
				require.Never(t, func() bool {
					return len(installer.GetInstalledInstances()) > 0 || reporter.ResourceCreateEventCount() > 0
				}, 500*time.Millisecond, 50*time.Millisecond)
			}
		})

	}
}

type mockGCPClient struct {
	vms []*gcpimds.Instance
}

func (m *mockGCPClient) getVMSForProject(projectID string) []*gcpimds.Instance {
	var vms []*gcpimds.Instance
	for _, vm := range m.vms {
		if vm.ProjectID == projectID {
			vms = append(vms, vm)
		}
	}
	return vms
}

func (m *mockGCPClient) ListInstances(_ context.Context, projectID, _ string) ([]*gcpimds.Instance, error) {
	return m.getVMSForProject(projectID), nil
}

func (m *mockGCPClient) StreamInstances(_ context.Context, projectID, _ string) stream.Stream[*gcpimds.Instance] {
	return stream.Slice(m.getVMSForProject(projectID))
}

func (m *mockGCPClient) GetInstance(_ context.Context, _ *gcpimds.InstanceRequest) (*gcpimds.Instance, error) {
	return nil, trace.NotFound("disabled for test")
}

func (m *mockGCPClient) GetInstanceTags(_ context.Context, _ *gcpimds.InstanceRequest) (map[string]string, error) {
	return nil, nil
}

func (m *mockGCPClient) AddSSHKey(_ context.Context, _ *gcp.SSHKeyRequest) error {
	return nil
}

func (m *mockGCPClient) RemoveSSHKey(_ context.Context, _ *gcp.SSHKeyRequest) error {
	return nil
}

type mockGCPInstaller struct {
	mu                 sync.Mutex
	installedInstances map[string]struct{}
}

func (m *mockGCPInstaller) Run(_ context.Context, req server.GCPRunRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range req.Instances {
		m.installedInstances[inst.Name] = struct{}{}
	}
	return nil
}

func (m *mockGCPInstaller) GetInstalledInstances() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]string, 0, len(m.installedInstances))
	for k := range m.installedInstances {
		keys = append(keys, k)
	}
	return keys
}

func TestGCPVMDiscovery(t *testing.T) {
	t.Parallel()

	defaultDiscoveryGroup := "dc001"
	defaultStaticMatcher := Matchers{
		GCP: []types.GCPMatcher{{
			Types:      []string{"gce"},
			ProjectIDs: []string{"myproject"},
			Locations:  []string{"myzone"},
			Labels:     types.Labels{"teleport": {"yes"}},
		}},
	}

	defaultDiscoveryConfig, err := discoveryconfig.NewDiscoveryConfig(
		header.Metadata{Name: uuid.NewString()},
		discoveryconfig.Spec{
			DiscoveryGroup: defaultDiscoveryGroup,
			AWS:            defaultStaticMatcher.AWS,
			Azure:          defaultStaticMatcher.Azure,
			GCP:            defaultStaticMatcher.GCP,
			Kube:           defaultStaticMatcher.Kubernetes,
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name                   string
		presentVMs             []types.Server
		foundGCPVMs            []*gcpimds.Instance
		discoveryConfig        *discoveryconfig.DiscoveryConfig
		staticMatchers         Matchers
		wantInstalledInstances []string
	}{
		{
			name:       "no nodes present, 1 found",
			presentVMs: []types.Server{},
			foundGCPVMs: []*gcpimds.Instance{
				{
					ProjectID: "myproject",
					Zone:      "myzone",
					Name:      "myinstance",
					Labels: map[string]string{
						"teleport": "yes",
					},
				},
			},
			staticMatchers:         defaultStaticMatcher,
			wantInstalledInstances: []string{"myinstance"},
		},
		{
			name:       "no nodes present, 2 found for different projects",
			presentVMs: []types.Server{},
			foundGCPVMs: []*gcpimds.Instance{
				{
					ProjectID: "p1",
					Zone:      "myzone",
					Name:      "myinstance1",
					Labels: map[string]string{
						"teleport": "yes",
					},
				},
				{
					ProjectID: "p2",
					Zone:      "myzone",
					Name:      "myinstance2",
					Labels: map[string]string{
						"teleport": "yes",
					},
				},
			},
			staticMatchers: Matchers{
				GCP: []types.GCPMatcher{{
					Types:      []string{"gce"},
					ProjectIDs: []string{"*"},
					Locations:  []string{"myzone"},
					Labels:     types.Labels{"teleport": {"yes"}},
				}},
			},
			wantInstalledInstances: []string{"myinstance1", "myinstance2"},
		},
		{
			name: "nodes present, instance filtered",
			presentVMs: []types.Server{
				&types.ServerV2{
					Kind: types.KindNode,
					Metadata: types.Metadata{
						Name: "name",
						Labels: map[string]string{
							"teleport.internal/project-id": "myproject",
							"teleport.internal/zone":       "myzone",
							"teleport.internal/name":       "myinstance",
						},
						Namespace: defaults.Namespace,
					},
				},
			},
			staticMatchers: defaultStaticMatcher,
			foundGCPVMs: []*gcpimds.Instance{
				{
					ProjectID: "myproject",
					Zone:      "myzone",
					Name:      "myinstance",
					Labels: map[string]string{
						"teleport": "yes",
					},
				},
			},
		},
		{
			name: "nodes present, instance not filtered",
			presentVMs: []types.Server{
				&types.ServerV2{
					Kind: types.KindNode,
					Metadata: types.Metadata{
						Name: "name",
						Labels: map[string]string{
							"teleport.internal/project-id": "myproject",
							"teleport.internal/zone":       "myzone",
							"teleport.internal/name":       "myotherinstance",
						},
						Namespace: defaults.Namespace,
					},
				},
			},
			staticMatchers: defaultStaticMatcher,
			foundGCPVMs: []*gcpimds.Instance{
				{
					ProjectID: "myproject",
					Zone:      "myzone",
					Name:      "myinstance",
					Labels: map[string]string{
						"teleport": "yes",
					},
				},
			},
			wantInstalledInstances: []string{"myinstance"},
		},
		{
			name:       "no nodes present, 1 found usind dynamic matchers",
			presentVMs: []types.Server{},
			foundGCPVMs: []*gcpimds.Instance{
				{
					ProjectID: "myproject",
					Zone:      "myzone",
					Name:      "myinstance",
					Labels: map[string]string{
						"teleport": "yes",
					},
				},
			},
			staticMatchers:         Matchers{},
			discoveryConfig:        defaultDiscoveryConfig,
			wantInstalledInstances: []string{"myinstance"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testCloudClients := &cloud.TestCloudClients{
				GCPInstances: &mockGCPClient{
					vms: tc.foundGCPVMs,
				},
				GCPProjects: newPopulatedGCPProjectsMock(),
			}

			ctx := context.Background()
			testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
				Dir: t.TempDir(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

			tlsServer, err := testAuthServer.NewTestTLSServer()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

			// Auth client for discovery service.
			identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
			authClient, err := tlsServer.NewClient(identity)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, authClient.Close()) })

			for _, instance := range tc.presentVMs {
				_, err := tlsServer.Auth().UpsertNode(ctx, instance)
				require.NoError(t, err)
			}

			logger := logtest.NewLogger()
			emitter := &mockEmitter{}
			reporter := &mockUsageReporter{}
			installer := &mockGCPInstaller{
				installedInstances: make(map[string]struct{}),
			}
			tlsServer.Auth().SetUsageReporter(reporter)
			server, err := New(authz.ContextWithUser(context.Background(), identity.I), &Config{
				CloudClients:     testCloudClients,
				ClusterFeatures:  func() proto.Features { return proto.Features{} },
				KubernetesClient: fake.NewSimpleClientset(),
				AccessPoint:      getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
				Matchers:         tc.staticMatchers,
				Emitter:          emitter,
				Log:              logger,
				DiscoveryGroup:   defaultDiscoveryGroup,
			})

			require.NoError(t, err)
			server.gcpInstaller = installer
			emitter.server = server
			emitter.t = t

			if tc.discoveryConfig != nil {
				_, err := tlsServer.Auth().DiscoveryConfigs.CreateDiscoveryConfig(ctx, tc.discoveryConfig)
				require.NoError(t, err)

				// Wait for the DiscoveryConfig to be added to the dynamic matchers
				require.Eventually(t, func() bool {
					server.muDynamicServerGCPFetchers.RLock()
					defer server.muDynamicServerGCPFetchers.RUnlock()
					return len(server.dynamicServerGCPFetchers) > 0
				}, 1*time.Second, 100*time.Millisecond)
			}

			go server.Start()
			t.Cleanup(server.Stop)

			if len(tc.wantInstalledInstances) > 0 {
				require.Eventually(t, func() bool {
					instances := installer.GetInstalledInstances()
					slices.Sort(instances)
					return slices.Equal(tc.wantInstalledInstances, instances) && len(tc.wantInstalledInstances) == reporter.ResourceCreateEventCount()
				}, 500*time.Millisecond, 50*time.Millisecond)
			} else {
				require.Never(t, func() bool {
					return len(installer.GetInstalledInstances()) > 0 || reporter.ResourceCreateEventCount() > 0
				}, 500*time.Millisecond, 50*time.Millisecond)
			}
		})
	}
}

// TestServer_onCreate tests the update of the discovery_group of a resource
// when a resource already exists with the same name but an empty discovery_group.
func TestServer_onCreate(t *testing.T) {
	accessPoint := &fakeAccessPoint{}
	s := &Server{
		Config: &Config{
			DiscoveryGroup: "test-cluster",
			AccessPoint:    accessPoint,
			Log:            logtest.NewLogger(),
		},
	}

	t.Run("onCreate update kube", func(t *testing.T) {
		// With cloud origin and an empty discovery group, it should update.
		accessPoint.kube = mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{})
		err := s.onKubeCreate(context.Background(), mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: "test-cluster"}))
		require.NoError(t, err)
		require.True(t, accessPoint.updatedKube)

		// Reset the updated flag and set the registered kube cluster to have
		// non-cloud origin. It should not update.
		accessPoint.updatedKube = false
		accessPoint.kube.SetOrigin(types.OriginDynamic)
		err = s.onKubeCreate(context.Background(), mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: "test-cluster"}))
		require.Error(t, err)
		require.False(t, accessPoint.updatedKube)

		// Reset the updated flag and set the registered kube cluster to have
		// an empty origin. It should not update.
		accessPoint.updatedKube = false
		accessPoint.kube.SetOrigin("")
		err = s.onKubeCreate(context.Background(), mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: "test-cluster"}))
		require.Error(t, err)
		require.False(t, accessPoint.updatedKube)

		// Reset the update flag and set the registered kube cluster to have
		// a non-empty discovery group. It should not update.
		accessPoint.updatedKube = false
		accessPoint.kube = mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: "non-empty"})
		err = s.onKubeCreate(context.Background(), mustConvertEKSToKubeCluster(t, eksMockClusters[0], rewriteDiscoveryLabelsParams{discoveryGroup: "test-cluster"}))
		require.Error(t, err)
		require.False(t, accessPoint.updatedKube)
	})

	t.Run("onCreate update database", func(t *testing.T) {
		_, awsRedshiftDB := makeRedshiftCluster(t, "aws-redshift", "us-east-1", rewriteDiscoveryLabelsParams{discoveryGroup: "test"})
		_, awsRedshiftDBEmptyDiscoveryGroup := makeRedshiftCluster(t, "aws-redshift", "us-east-1", rewriteDiscoveryLabelsParams{})

		// With cloud origin and an empty discovery group, it should update.
		accessPoint.database = awsRedshiftDBEmptyDiscoveryGroup
		err := s.onDatabaseCreate(context.Background(), awsRedshiftDB)
		require.NoError(t, err)
		require.True(t, accessPoint.updatedDatabase)

		// Reset the updated flag and set the db to empty discovery group
		// but non-cloud origin. It should not update.
		accessPoint.updatedDatabase = false
		accessPoint.database.SetOrigin(types.OriginDynamic)
		err = s.onDatabaseCreate(context.Background(), awsRedshiftDB)
		require.Error(t, err)
		require.False(t, accessPoint.updatedDatabase)

		// Reset the updated flag and set the db to empty discovery group
		// but empty origin. It should not update.
		accessPoint.updatedDatabase = false
		accessPoint.database.SetOrigin("")
		err = s.onDatabaseCreate(context.Background(), awsRedshiftDB)
		require.Error(t, err)
		require.False(t, accessPoint.updatedDatabase)

		// Reset the updated flag and set the registered db to have a non-empty
		// discovery group. It should not update.
		accessPoint.updatedDatabase = false
		accessPoint.database = awsRedshiftDB
		err = s.onDatabaseCreate(context.Background(), awsRedshiftDB)
		require.Error(t, err)
		require.False(t, accessPoint.updatedDatabase)
	})

	t.Run("onCreate update app", func(t *testing.T) {
		kubeSvc := newMockKubeService("service1", "ns1", "",
			map[string]string{"test-label": "testval"}, nil,
			[]corev1.ServicePort{{Port: 42, Name: "http", Protocol: corev1.ProtocolTCP}})

		// With kube origin and empty discovery group, it should update.
		accessPoint.app = mustConvertKubeServiceToApp(t, "" /*empty discovery group*/, "http", kubeSvc, kubeSvc.Spec.Ports[0])
		err := s.onAppCreate(context.Background(), mustConvertKubeServiceToApp(t, "notEmpty", "http", kubeSvc, kubeSvc.Spec.Ports[0]))
		require.NoError(t, err)
		require.True(t, accessPoint.updatedApp)

		// Reset the updated flag and set the app to empty discovery group
		// but non-cloud origin. It should not update.
		accessPoint.updatedApp = false
		accessPoint.app.SetOrigin(types.OriginDynamic)
		err = s.onAppCreate(context.Background(), mustConvertKubeServiceToApp(t, "notEmpty", "http", kubeSvc, kubeSvc.Spec.Ports[0]))
		require.Error(t, err)
		require.False(t, accessPoint.updatedApp)

		// Reset the updated flag and set the app to empty discovery group
		// but non-cloud origin. It should not update.
		accessPoint.updatedApp = false
		accessPoint.app.SetOrigin("")
		err = s.onAppCreate(context.Background(), mustConvertKubeServiceToApp(t, "notEmpty", "http", kubeSvc, kubeSvc.Spec.Ports[0]))
		require.Error(t, err)
		require.False(t, accessPoint.updatedApp)

		// Reset the updated flag and set the app to non-empty discovery group.
		// It should not update.
		accessPoint.updatedApp = false
		accessPoint.app = mustConvertKubeServiceToApp(t, "nonEmpty", "http", kubeSvc, kubeSvc.Spec.Ports[0])
		err = s.onAppCreate(context.Background(), mustConvertKubeServiceToApp(t, "notEmpty", "http", kubeSvc, kubeSvc.Spec.Ports[0]))
		require.Error(t, err)
		require.False(t, accessPoint.updatedApp)
	})
}

func TestEmitUsageEvents(t *testing.T) {
	t.Parallel()
	testClients := cloud.TestCloudClients{
		AzureVirtualMachines: &mockAzureClient{},
		AzureRunCommand:      &mockAzureRunCommandClient{},
	}
	testAuthServer, err := authtest.NewAuthServer(authtest.AuthServerConfig{
		Dir: t.TempDir(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testAuthServer.Close()) })

	tlsServer, err := testAuthServer.NewTestTLSServer()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tlsServer.Close()) })

	// Auth client for discovery service.
	identity := authtest.TestServerID(types.RoleDiscovery, "hostID")
	authClient, err := tlsServer.NewClient(identity)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, authClient.Close()) })

	reporter := &mockUsageReporter{}
	tlsServer.Auth().SetUsageReporter(reporter)

	server, err := New(authz.ContextWithUser(context.Background(), identity.I), &Config{
		CloudClients:    &testClients,
		ClusterFeatures: func() proto.Features { return proto.Features{} },
		AccessPoint:     getDiscoveryAccessPoint(tlsServer.Auth(), authClient),
		Matchers: Matchers{
			Azure: []types.AzureMatcher{{
				Types:          []string{"vm"},
				Subscriptions:  []string{"testsub"},
				ResourceGroups: []string{"testrg"},
				Regions:        []string{"westcentralus"},
				ResourceTags:   types.Labels{"teleport": {"yes"}},
			}},
		},
		Emitter: &mockEmitter{},
	})
	require.NoError(t, err)

	require.Equal(t, 0, reporter.ResourceCreateEventCount())
	// Check that events are emitted for new instances.
	event := &usageeventsv1.ResourceCreateEvent{}
	require.NoError(t, server.emitUsageEvents(map[string]*usageeventsv1.ResourceCreateEvent{
		"inst1": event,
		"inst2": event,
	}))
	require.Equal(t, 2, reporter.ResourceCreateEventCount())
	// Check that events for duplicate instances are discarded.
	require.NoError(t, server.emitUsageEvents(map[string]*usageeventsv1.ResourceCreateEvent{
		"inst1": event,
		"inst3": event,
	}))
	require.Equal(t, 3, reporter.ResourceCreateEventCount())
}

type eksClustersEnroller interface {
	EnrollEKSClusters(context.Context, *integrationpb.EnrollEKSClustersRequest, ...grpc.CallOption) (*integrationpb.EnrollEKSClustersResponse, error)
}

type combinedDiscoveryClient struct {
	*auth.Server
	eksClustersEnroller
	discoveryConfigStatusUpdater interface {
		UpdateDiscoveryConfigStatus(context.Context, string, discoveryconfig.Status) (*discoveryconfig.DiscoveryConfig, error)
	}
}

func (d *combinedDiscoveryClient) EnrollEKSClusters(ctx context.Context, req *integrationpb.EnrollEKSClustersRequest, _ ...grpc.CallOption) (*integrationpb.EnrollEKSClustersResponse, error) {
	if d.eksClustersEnroller != nil {
		return d.eksClustersEnroller.EnrollEKSClusters(ctx, req)
	}
	return nil, trace.BadParameter("not implemented.")
}

func (d *combinedDiscoveryClient) UpdateDiscoveryConfigStatus(ctx context.Context, name string, status discoveryconfig.Status) (*discoveryconfig.DiscoveryConfig, error) {
	if d.discoveryConfigStatusUpdater != nil {
		return d.discoveryConfigStatusUpdater.UpdateDiscoveryConfigStatus(ctx, name, status)
	}
	return nil, trace.BadParameter("not implemented.")
}

func getDiscoveryAccessPointWithEKSEnroller(authServer *auth.Server, authClient authclient.ClientI, eksEnroller eksClustersEnroller) authclient.DiscoveryAccessPoint {
	return &combinedDiscoveryClient{Server: authServer, eksClustersEnroller: eksEnroller, discoveryConfigStatusUpdater: authClient.DiscoveryConfigClient()}
}

func getDiscoveryAccessPoint(authServer *auth.Server, authClient authclient.ClientI) authclient.DiscoveryAccessPoint {
	return &combinedDiscoveryClient{Server: authServer, eksClustersEnroller: authClient.IntegrationAWSOIDCClient(), discoveryConfigStatusUpdater: authClient.DiscoveryConfigClient()}
}

type fakeAccessPoint struct {
	authclient.DiscoveryAccessPoint

	ping              func(context.Context) (proto.PingResponse, error)
	enrollEKSClusters func(context.Context, *integrationpb.EnrollEKSClustersRequest, ...grpc.CallOption) (*integrationpb.EnrollEKSClustersResponse, error)

	updatedKube         bool
	updatedDatabase     bool
	updatedApp          bool
	kube                types.KubeCluster
	database            types.Database
	app                 types.Application
	upsertedServerInfos chan types.ServerInfo
	reports             map[string][]discoveryconfig.Status
}

func (f *fakeAccessPoint) UpdateDiscoveryConfigStatus(ctx context.Context, name string, status discoveryconfig.Status) (*discoveryconfig.DiscoveryConfig, error) {
	f.reports[name] = append(f.reports[name], status)
	return nil, nil
}

func newFakeAccessPoint() *fakeAccessPoint {
	return &fakeAccessPoint{
		upsertedServerInfos: make(chan types.ServerInfo),
		reports:             make(map[string][]discoveryconfig.Status),
	}
}

func (f *fakeAccessPoint) Ping(ctx context.Context) (proto.PingResponse, error) {
	if f.ping != nil {
		return f.ping(ctx)
	}
	return proto.PingResponse{}, trace.NotImplemented("not implemented")
}

func (f *fakeAccessPoint) EnrollEKSClusters(ctx context.Context, req *integrationpb.EnrollEKSClustersRequest, _ ...grpc.CallOption) (*integrationpb.EnrollEKSClustersResponse, error) {
	if f.enrollEKSClusters != nil {
		return f.enrollEKSClusters(ctx, req)
	}
	if f.DiscoveryAccessPoint != nil {
		return f.DiscoveryAccessPoint.EnrollEKSClusters(ctx, req)
	}
	return &integrationpb.EnrollEKSClustersResponse{}, trace.NotImplemented("not implemented")
}

func (f *fakeAccessPoint) GetKubernetesCluster(ctx context.Context, name string) (types.KubeCluster, error) {
	return f.kube, nil
}

func (f *fakeAccessPoint) GetDatabase(ctx context.Context, name string) (types.Database, error) {
	return f.database, nil
}

func (f *fakeAccessPoint) CreateDatabase(ctx context.Context, database types.Database) error {
	return trace.AlreadyExists("already exists")
}

func (f *fakeAccessPoint) UpdateDatabase(ctx context.Context, database types.Database) error {
	f.updatedDatabase = true
	return nil
}

func (f *fakeAccessPoint) CreateKubernetesCluster(ctx context.Context, cluster types.KubeCluster) error {
	return trace.AlreadyExists("already exists")
}

// UpdateKubernetesCluster updates existing kubernetes cluster resource.
func (f *fakeAccessPoint) UpdateKubernetesCluster(ctx context.Context, cluster types.KubeCluster) error {
	f.updatedKube = true
	return nil
}

func (f *fakeAccessPoint) GetApp(ctx context.Context, name string) (types.Application, error) {
	return f.app, nil
}

func (f *fakeAccessPoint) CreateApp(ctx context.Context, _ types.Application) error {
	return trace.AlreadyExists("already exists")
}

func (f *fakeAccessPoint) UpdateApp(ctx context.Context, _ types.Application) error {
	f.updatedApp = true
	return nil
}

func (f *fakeAccessPoint) UpsertServerInfo(ctx context.Context, si types.ServerInfo) error {
	f.upsertedServerInfos <- si
	return nil
}

func (f *fakeAccessPoint) NewWatcher(ctx context.Context, watch types.Watch) (types.Watcher, error) {
	if f.DiscoveryAccessPoint != nil {
		return f.DiscoveryAccessPoint.NewWatcher(ctx, watch)
	}
	return newFakeWatcher(), nil
}

type fakeWatcher struct{}

func newFakeWatcher() fakeWatcher {
	return fakeWatcher{}
}

func (m fakeWatcher) Events() <-chan types.Event {
	return make(chan types.Event)
}

func (m fakeWatcher) Done() <-chan struct{} {
	return make(chan struct{})
}

func (m fakeWatcher) Close() error {
	return nil
}

func (m fakeWatcher) Error() error {
	return nil
}

type rewriteDiscoveryLabelsParams struct {
	matcherType         string
	discoveryConfigName string
	discoveryGroup      string
	integration         string
}

// rewriteCloudResource is a test helper func that rewrites an expected cloud
// resource to include all the metadata we expect to be added by discovery.
func rewriteCloudResource(t *testing.T, r types.ResourceWithLabels, discoveryParams rewriteDiscoveryLabelsParams) {
	r.SetOrigin(types.OriginCloud)
	staticLabels := r.GetStaticLabels()
	if discoveryParams.matcherType != "" {
		staticLabels[types.DiscoveryTypeLabel] = discoveryParams.matcherType
	}
	if discoveryParams.discoveryConfigName != "" {
		staticLabels[types.TeleportInternalDiscoveryConfigName] = discoveryParams.discoveryConfigName
	}
	if discoveryParams.discoveryGroup != "" {
		staticLabels[types.TeleportInternalDiscoveryGroupName] = discoveryParams.discoveryGroup
	}
	if discoveryParams.integration != "" {
		staticLabels[types.TeleportInternalDiscoveryIntegrationName] = discoveryParams.integration
	}
	r.SetStaticLabels(staticLabels)

	switch r := r.(type) {
	case types.Database:
		cloudLabel, ok := r.GetLabel(types.CloudLabel)
		require.True(t, ok, "cloud resources should have a label identifying the cloud they came from")
		switch cloudLabel {
		case types.CloudAWS:
			common.ApplyAWSDatabaseNameSuffix(r, discoveryParams.matcherType)
		case types.CloudAzure:
			common.ApplyAzureDatabaseNameSuffix(r, discoveryParams.matcherType)
		case types.CloudGCP:
			require.FailNow(t, "GCP database discovery is not supported", cloudLabel)
		default:
			require.FailNow(t, "unknown cloud label %q", cloudLabel)
		}
	case types.KubeCluster:
		cloudLabel, ok := r.GetLabel(types.CloudLabel)
		require.True(t, ok, "cloud resources should have a label identifying the cloud they came from")
		switch cloudLabel {
		case types.CloudAWS:
			common.ApplyEKSNameSuffix(r)
		case types.CloudAzure:
			common.ApplyAKSNameSuffix(r)
		case types.CloudGCP:
			common.ApplyGKENameSuffix(r)
		default:
			require.FailNow(t, "unknown cloud label %q", cloudLabel)
		}
	default:
		require.FailNow(t, "unknown cloud resource type %T", r)
	}
}

type mockProjectsAPI struct {
	gcp.ProjectsClient
	projects []gcp.Project
}

func (m *mockProjectsAPI) ListProjects(ctx context.Context) ([]gcp.Project, error) {
	return m.projects, nil
}

func newPopulatedGCPProjectsMock() *mockProjectsAPI {
	return &mockProjectsAPI{
		projects: []gcp.Project{
			{
				ID:   "p1",
				Name: "project1",
			},
			{
				ID:   "p2",
				Name: "project2",
			},
		},
	}
}

type fakeAWSClients struct {
	ecClient         db.ElastiCacheClient
	mdbClient        db.MemoryDBClient
	openSearchClient db.OpenSearchClient
	rdsClient        db.RDSClient
	redshiftClient   db.RedshiftClient
	rssClient        db.RSSClient
}

func (f fakeAWSClients) GetElastiCacheClient(cfg aws.Config, optFns ...func(*elasticache.Options)) db.ElastiCacheClient {
	return f.ecClient
}

func (f fakeAWSClients) GetMemoryDBClient(cfg aws.Config, optFns ...func(*memorydb.Options)) db.MemoryDBClient {
	return f.mdbClient
}

func (f fakeAWSClients) GetOpenSearchClient(cfg aws.Config, optFns ...func(*opensearch.Options)) db.OpenSearchClient {
	return f.openSearchClient
}

func (f fakeAWSClients) GetRDSClient(cfg aws.Config, optFns ...func(*rds.Options)) db.RDSClient {
	return f.rdsClient
}

func (f fakeAWSClients) GetRedshiftClient(cfg aws.Config, optFns ...func(*redshift.Options)) db.RedshiftClient {
	return f.redshiftClient
}

func (f fakeAWSClients) GetRedshiftServerlessClient(cfg aws.Config, optFns ...func(*rss.Options)) db.RSSClient {
	return f.rssClient
}
