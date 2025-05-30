// Code generated by _gen/main.go DO NOT EDIT
/*
Copyright 2015-2024 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"context"
	"fmt"
	apitypes "github.com/gravitational/teleport/api/types"

	autoupdatev1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/autoupdate/v1"
	headerv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/header/v1"
	"github.com/gravitational/teleport/integrations/lib/backoff"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonboulle/clockwork"

	schemav1 "github.com/gravitational/teleport/integrations/terraform/tfschema/autoupdate/v1"
)

// resourceTeleportAutoUpdateVersionType is the resource metadata type
type resourceTeleportAutoUpdateVersionType struct{}

// resourceTeleportAutoUpdateVersion is the resource
type resourceTeleportAutoUpdateVersion struct {
	p Provider
}

// GetSchema returns the resource schema
func (r resourceTeleportAutoUpdateVersionType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return schemav1.GenSchemaAutoUpdateVersion(ctx)
}

// NewResource creates the empty resource
func (r resourceTeleportAutoUpdateVersionType) NewResource(_ context.Context, p tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	return resourceTeleportAutoUpdateVersion{
		p: *(p.(*Provider)),
	}, nil
}

// Create creates the AutoUpdateVersion
func (r resourceTeleportAutoUpdateVersion) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	autoUpdateVersion := &autoupdatev1.AutoUpdateVersion{}
	diags = schemav1.CopyAutoUpdateVersionFromTerraform(ctx, plan, autoUpdateVersion)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	autoUpdateVersion.Kind = apitypes.KindAutoUpdateVersion

	
	if autoUpdateVersion.GetMetadata() == nil {
		autoUpdateVersion.Metadata = &headerv1.Metadata{}
	}
	if autoUpdateVersion.GetMetadata().GetName() == "" {
		autoUpdateVersion.Metadata.Name = apitypes.MetaNameAutoUpdateVersion
	}

	autoUpdateVersionBefore, err := r.p.Client.GetAutoUpdateVersion(ctx)
	if err != nil && !trace.IsNotFound(err) {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	_, err = r.p.Client.CreateAutoUpdateVersion(ctx, autoUpdateVersion)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error creating AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	// Not really an inferface, just using the same name for easier templating.
	var autoUpdateVersionI *autoupdatev1.AutoUpdateVersion
	

	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		autoUpdateVersionI, err = r.p.Client.GetAutoUpdateVersion(ctx)
		if err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
			return
		}

		previousMetadata := autoUpdateVersionBefore.GetMetadata()
		currentMetadata := autoUpdateVersionI.GetMetadata()
		if previousMetadata.GetRevision() != currentMetadata.GetRevision() || false {
			break
		}
		if bErr := backoff.Do(ctx); bErr != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(bErr), "autoupdate_version"))
			return
		}
		if tries >= r.p.RetryConfig.MaxTries {
			diagMessage := fmt.Sprintf("Error reading AutoUpdateVersion (tried %d times) - state outdated, please import resource", tries)
			resp.Diagnostics.AddError(diagMessage, "autoupdate_version")
			return
		}
	}
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	autoUpdateVersion = autoUpdateVersionI
	

	diags = schemav1.CopyAutoUpdateVersionToTerraform(ctx, autoUpdateVersion, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Attrs["id"] = types.String{Value: autoUpdateVersion.Metadata.Name}

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read reads teleport AutoUpdateVersion
func (r resourceTeleportAutoUpdateVersion) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var state types.Object
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	autoUpdateVersionI, err := r.p.Client.GetAutoUpdateVersion(ctx)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	autoUpdateVersion := autoUpdateVersionI
	diags = schemav1.CopyAutoUpdateVersionToTerraform(ctx, autoUpdateVersion, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates teleport AutoUpdateVersion
func (r resourceTeleportAutoUpdateVersion) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	autoUpdateVersion := &autoupdatev1.AutoUpdateVersion{}
	diags = schemav1.CopyAutoUpdateVersionFromTerraform(ctx, plan, autoUpdateVersion)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	autoUpdateVersionBefore, err := r.p.Client.GetAutoUpdateVersion(ctx)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	_, err = r.p.Client.UpsertAutoUpdateVersion(ctx, autoUpdateVersion)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}
	var autoUpdateVersionI *autoupdatev1.AutoUpdateVersion

	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		autoUpdateVersionI, err = r.p.Client.GetAutoUpdateVersion(ctx)
		if err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
			return
		}
		if autoUpdateVersionBefore.GetMetadata().Revision != autoUpdateVersionI.GetMetadata().Revision || false {
			break
		}
		if bErr := backoff.Do(ctx); bErr != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(bErr), "autoupdate_version"))
			return
		}
		if tries >= r.p.RetryConfig.MaxTries {
			diagMessage := fmt.Sprintf("Error reading AutoUpdateVersion (tried %d times) - state outdated, please import resource", tries)
			resp.Diagnostics.AddError(diagMessage, "autoupdate_version")
			return
		}
	}
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	autoUpdateVersion = autoUpdateVersionI

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes Teleport AutoUpdateVersion
func (r resourceTeleportAutoUpdateVersion) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	err := r.p.Client.DeleteAutoUpdateVersion(ctx)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error deleting AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}

	resp.State.RemoveResource(ctx)
}

// ImportState imports AutoUpdateVersion state
func (r resourceTeleportAutoUpdateVersion) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	autoUpdateVersionI, err := r.p.Client.GetAutoUpdateVersion(ctx)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating AutoUpdateVersion", trace.Wrap(err), "autoupdate_version"))
		return
	}
	autoUpdateVersion := autoUpdateVersionI

	var state types.Object

	diags := resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = schemav1.CopyAutoUpdateVersionToTerraform(ctx, autoUpdateVersion, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := autoUpdateVersion.Metadata.Name

	state.Attrs["id"] = types.String{Value: id}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
