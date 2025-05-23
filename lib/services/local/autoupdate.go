/*
 * Teleport
 * Copyright (C) 2024  Gravitational, Inc.
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

package local

import (
	"context"

	"github.com/gravitational/trace"

	"github.com/gravitational/teleport/api/gen/proto/go/teleport/autoupdate/v1"
	"github.com/gravitational/teleport/api/types"
	update "github.com/gravitational/teleport/api/types/autoupdate"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/services/local/generic"
)

const (
	autoUpdateConfigPrefix       = "auto_update_config"
	autoUpdateVersionPrefix      = "auto_update_version"
	autoUpdateAgentRolloutPrefix = "auto_update_agent_rollout"
	autoUpdateAgentReportPrefix  = "auto_update_agent_report"
)

// AutoUpdateService is responsible for managing AutoUpdateConfig and AutoUpdateVersion singleton resources.
type AutoUpdateService struct {
	config  *generic.ServiceWrapper[*autoupdate.AutoUpdateConfig]
	version *generic.ServiceWrapper[*autoupdate.AutoUpdateVersion]
	rollout *generic.ServiceWrapper[*autoupdate.AutoUpdateAgentRollout]
	report  *generic.ServiceWrapper[*autoupdate.AutoUpdateAgentReport]
}

// NewAutoUpdateService returns a new AutoUpdateService.
func NewAutoUpdateService(b backend.Backend) (*AutoUpdateService, error) {
	config, err := generic.NewServiceWrapper(
		generic.ServiceConfig[*autoupdate.AutoUpdateConfig]{
			Backend:       b,
			ResourceKind:  types.KindAutoUpdateConfig,
			BackendPrefix: backend.NewKey(autoUpdateConfigPrefix),
			MarshalFunc:   services.MarshalProtoResource[*autoupdate.AutoUpdateConfig],
			UnmarshalFunc: services.UnmarshalProtoResource[*autoupdate.AutoUpdateConfig],
			ValidateFunc:  update.ValidateAutoUpdateConfig,
			NameKeyFunc: func(string) string {
				return types.MetaNameAutoUpdateConfig
			},
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	version, err := generic.NewServiceWrapper(
		generic.ServiceConfig[*autoupdate.AutoUpdateVersion]{
			Backend:       b,
			ResourceKind:  types.KindAutoUpdateVersion,
			BackendPrefix: backend.NewKey(autoUpdateVersionPrefix),
			MarshalFunc:   services.MarshalProtoResource[*autoupdate.AutoUpdateVersion],
			UnmarshalFunc: services.UnmarshalProtoResource[*autoupdate.AutoUpdateVersion],
			ValidateFunc:  update.ValidateAutoUpdateVersion,
			NameKeyFunc: func(string) string {
				return types.MetaNameAutoUpdateVersion
			},
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	rollout, err := generic.NewServiceWrapper(
		generic.ServiceConfig[*autoupdate.AutoUpdateAgentRollout]{
			Backend:       b,
			ResourceKind:  types.KindAutoUpdateAgentRollout,
			BackendPrefix: backend.NewKey(autoUpdateAgentRolloutPrefix),
			MarshalFunc:   services.MarshalProtoResource[*autoupdate.AutoUpdateAgentRollout],
			UnmarshalFunc: services.UnmarshalProtoResource[*autoupdate.AutoUpdateAgentRollout],
			ValidateFunc:  update.ValidateAutoUpdateAgentRollout,
			NameKeyFunc: func(string) string {
				return types.MetaNameAutoUpdateAgentRollout
			},
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	report, err := generic.NewServiceWrapper(
		generic.ServiceConfig[*autoupdate.AutoUpdateAgentReport]{
			Backend:       b,
			ResourceKind:  types.KindAutoUpdateAgentRollout,
			BackendPrefix: backend.NewKey(autoUpdateAgentReportPrefix),
			MarshalFunc:   services.MarshalProtoResource[*autoupdate.AutoUpdateAgentReport],
			UnmarshalFunc: services.UnmarshalProtoResource[*autoupdate.AutoUpdateAgentReport],
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &AutoUpdateService{
		config:  config,
		version: version,
		rollout: rollout,
		report:  report,
	}, nil
}

// CreateAutoUpdateConfig creates the AutoUpdateConfig singleton resource.
func (s *AutoUpdateService) CreateAutoUpdateConfig(
	ctx context.Context,
	c *autoupdate.AutoUpdateConfig,
) (*autoupdate.AutoUpdateConfig, error) {
	config, err := s.config.CreateResource(ctx, c)
	return config, trace.Wrap(err)
}

// UpdateAutoUpdateConfig updates the AutoUpdateConfig singleton resource.
func (s *AutoUpdateService) UpdateAutoUpdateConfig(
	ctx context.Context,
	c *autoupdate.AutoUpdateConfig,
) (*autoupdate.AutoUpdateConfig, error) {
	config, err := s.config.ConditionalUpdateResource(ctx, c)
	return config, trace.Wrap(err)
}

// UpsertAutoUpdateConfig sets the AutoUpdateConfig singleton resource.
func (s *AutoUpdateService) UpsertAutoUpdateConfig(
	ctx context.Context,
	c *autoupdate.AutoUpdateConfig,
) (*autoupdate.AutoUpdateConfig, error) {
	config, err := s.config.UpsertResource(ctx, c)
	return config, trace.Wrap(err)
}

// GetAutoUpdateConfig gets the AutoUpdateConfig singleton resource.
func (s *AutoUpdateService) GetAutoUpdateConfig(ctx context.Context) (*autoupdate.AutoUpdateConfig, error) {
	config, err := s.config.GetResource(ctx, types.MetaNameAutoUpdateConfig)
	return config, trace.Wrap(err)
}

// DeleteAutoUpdateConfig deletes the AutoUpdateConfig singleton resource.
func (s *AutoUpdateService) DeleteAutoUpdateConfig(ctx context.Context) error {
	return trace.Wrap(s.config.DeleteResource(ctx, types.MetaNameAutoUpdateConfig))
}

// CreateAutoUpdateVersion creates the AutoUpdateVersion singleton resource.
func (s *AutoUpdateService) CreateAutoUpdateVersion(
	ctx context.Context,
	v *autoupdate.AutoUpdateVersion,
) (*autoupdate.AutoUpdateVersion, error) {
	version, err := s.version.CreateResource(ctx, v)
	return version, trace.Wrap(err)
}

// UpdateAutoUpdateVersion updates the AutoUpdateVersion singleton resource.
func (s *AutoUpdateService) UpdateAutoUpdateVersion(
	ctx context.Context,
	v *autoupdate.AutoUpdateVersion,
) (*autoupdate.AutoUpdateVersion, error) {
	version, err := s.version.ConditionalUpdateResource(ctx, v)
	return version, trace.Wrap(err)
}

// UpsertAutoUpdateVersion sets the AutoUpdateVersion singleton resource.
func (s *AutoUpdateService) UpsertAutoUpdateVersion(
	ctx context.Context,
	v *autoupdate.AutoUpdateVersion,
) (*autoupdate.AutoUpdateVersion, error) {
	version, err := s.version.UpsertResource(ctx, v)
	return version, trace.Wrap(err)
}

// GetAutoUpdateVersion gets the AutoUpdateVersion singleton resource.
func (s *AutoUpdateService) GetAutoUpdateVersion(ctx context.Context) (*autoupdate.AutoUpdateVersion, error) {
	version, err := s.version.GetResource(ctx, types.MetaNameAutoUpdateVersion)
	return version, trace.Wrap(err)
}

// DeleteAutoUpdateVersion deletes the AutoUpdateVersion singleton resource.
func (s *AutoUpdateService) DeleteAutoUpdateVersion(ctx context.Context) error {
	return trace.Wrap(s.version.DeleteResource(ctx, types.MetaNameAutoUpdateVersion))
}

// CreateAutoUpdateAgentRollout creates the AutoUpdateAgentRollout singleton resource.
func (s *AutoUpdateService) CreateAutoUpdateAgentRollout(
	ctx context.Context,
	v *autoupdate.AutoUpdateAgentRollout,
) (*autoupdate.AutoUpdateAgentRollout, error) {
	rollout, err := s.rollout.CreateResource(ctx, v)
	return rollout, trace.Wrap(err)
}

// UpdateAutoUpdateAgentRollout updates the AutoUpdateAgentRollout singleton resource.
func (s *AutoUpdateService) UpdateAutoUpdateAgentRollout(
	ctx context.Context,
	v *autoupdate.AutoUpdateAgentRollout,
) (*autoupdate.AutoUpdateAgentRollout, error) {
	rollout, err := s.rollout.ConditionalUpdateResource(ctx, v)
	return rollout, trace.Wrap(err)
}

// UpsertAutoUpdateAgentRollout sets the AutoUpdateAgentRollout singleton resource.
func (s *AutoUpdateService) UpsertAutoUpdateAgentRollout(
	ctx context.Context,
	v *autoupdate.AutoUpdateAgentRollout,
) (*autoupdate.AutoUpdateAgentRollout, error) {
	rollout, err := s.rollout.UpsertResource(ctx, v)
	return rollout, trace.Wrap(err)
}

// GetAutoUpdateAgentRollout gets the AutoUpdateAgentRollout singleton resource.
func (s *AutoUpdateService) GetAutoUpdateAgentRollout(ctx context.Context) (*autoupdate.AutoUpdateAgentRollout, error) {
	rollout, err := s.rollout.GetResource(ctx, types.MetaNameAutoUpdateAgentRollout)
	return rollout, trace.Wrap(err)
}

// DeleteAutoUpdateAgentRollout deletes the AutoUpdateAgentRollout singleton resource.
func (s *AutoUpdateService) DeleteAutoUpdateAgentRollout(ctx context.Context) error {
	return trace.Wrap(s.rollout.DeleteResource(ctx, types.MetaNameAutoUpdateAgentRollout))
}

// ListAutoUpdateAgentReports returns a paginated list of AutoUpdateAgentReport resources.
func (s *AutoUpdateService) ListAutoUpdateAgentReports(ctx context.Context, pageSize int, pageToken string) ([]*autoupdate.AutoUpdateAgentReport, string, error) {
	agentReports, nextKey, err := s.report.ListResources(ctx, pageSize, pageToken)
	return agentReports, nextKey, trace.Wrap(err)
}

// GetAutoUpdateAgentReport returns the specified AutoUpdateAgentReport resource.
func (s *AutoUpdateService) GetAutoUpdateAgentReport(ctx context.Context, name string) (*autoupdate.AutoUpdateAgentReport, error) {
	agentReport, err := s.report.GetResource(ctx, name)
	return agentReport, trace.Wrap(err)
}

// CreateAutoUpdateAgentReport creates a new AutoUpdateAgentReport resource.
func (s *AutoUpdateService) CreateAutoUpdateAgentReport(ctx context.Context, agentReport *autoupdate.AutoUpdateAgentReport) (*autoupdate.AutoUpdateAgentReport, error) {
	if err := update.ValidateAutoUpdateAgentReport(agentReport); err != nil {
		return nil, trace.Wrap(err, "validating autoupdate agent report")
	}
	created, err := s.report.CreateResource(ctx, agentReport)
	return created, trace.Wrap(err)
}

// UpdateAutoUpdateAgentReport updates an existing AutoUpdateAgentReport resource.
func (s *AutoUpdateService) UpdateAutoUpdateAgentReport(ctx context.Context, agentReport *autoupdate.AutoUpdateAgentReport) (*autoupdate.AutoUpdateAgentReport, error) {
	if err := update.ValidateAutoUpdateAgentReport(agentReport); err != nil {
		return nil, trace.Wrap(err, "validating autoupdate agent report")
	}
	updated, err := s.report.ConditionalUpdateResource(ctx, agentReport)
	return updated, trace.Wrap(err)
}

// UpsertAutoUpdateAgentReport upserts a AutoUpdateAgentReport resource.
func (s *AutoUpdateService) UpsertAutoUpdateAgentReport(ctx context.Context, agentReport *autoupdate.AutoUpdateAgentReport) (*autoupdate.AutoUpdateAgentReport, error) {
	if err := update.ValidateAutoUpdateAgentReport(agentReport); err != nil {
		return nil, trace.Wrap(err, "validating autoupdate agent report")
	}
	upserted, err := s.report.UpsertResource(ctx, agentReport)
	return upserted, trace.Wrap(err)
}

// DeleteAutoUpdateAgentReport removes the specified AutoUpdateAgentReport resource.
func (s *AutoUpdateService) DeleteAutoUpdateAgentReport(ctx context.Context, name string) error {
	return trace.Wrap(s.report.DeleteResource(ctx, name))
}

// DeleteAllAutoUpdateAgentReports removes all AutoUpdateAgentReport resources.
func (s *AutoUpdateService) DeleteAllAutoUpdateAgentReports(ctx context.Context) error {
	return trace.Wrap(s.report.DeleteAllResources(ctx))
}

// itemFromAutoUpdateConfig generates `backend.Item` from `AutoUpdateConfig` resource type.
func itemFromAutoUpdateConfig(config *autoupdate.AutoUpdateConfig) (*backend.Item, error) {
	if err := update.ValidateAutoUpdateConfig(config); err != nil {
		return nil, trace.Wrap(err)
	}
	rev, err := types.GetRevision(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	value, err := services.MarshalProtoResource(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	expires, err := types.GetExpiry(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	item := &backend.Item{
		Key:      backend.NewKey(autoUpdateConfigPrefix).AppendKey(backend.NewKey(types.MetaNameAutoUpdateConfig)),
		Value:    value,
		Expires:  expires,
		Revision: rev,
	}
	return item, nil
}

// itemFromAutoUpdateVersion generates `backend.Item` from `AutoUpdateVersion` resource type.
func itemFromAutoUpdateVersion(version *autoupdate.AutoUpdateVersion) (*backend.Item, error) {
	if err := update.ValidateAutoUpdateVersion(version); err != nil {
		return nil, trace.Wrap(err)
	}
	rev, err := types.GetRevision(version)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	value, err := services.MarshalProtoResource(version)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	expires, err := types.GetExpiry(version)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	item := &backend.Item{
		Key:      backend.NewKey(autoUpdateVersionPrefix).AppendKey(backend.NewKey(types.MetaNameAutoUpdateVersion)),
		Value:    value,
		Expires:  expires,
		Revision: rev,
	}
	return item, nil
}
