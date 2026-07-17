package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bitnixdev/terraform-provider-skipjack/internal/client"
)

var (
	_ datasource.DataSource              = &SecretDataSource{}
	_ datasource.DataSourceWithConfigure = &SecretDataSource{}
)

// SecretDataSource reads one secret (like vault_kv_secret_v2 data source).
type SecretDataSource struct {
	client *client.Client
}

type secretDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Org     types.String `tfsdk:"org"`
	Project types.String `tfsdk:"project"`
	Name    types.String `tfsdk:"name"`
	Value   types.String `tfsdk:"value"`
	Version types.Int64  `tfsdk:"version"`
}

func NewSecretDataSource() datasource.DataSource {
	return &SecretDataSource{}
}

func (d *SecretDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (d *SecretDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single secret from Skipjack by name. " +
			"Requires a token API key with `secretsRead`. " +
			"Comparable to the Vault `vault_kv_secret_v2` data source for a single value.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier: `org/project/name` or `org//name`.",
			},
			"org": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Skipjack organization slug.",
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Omit for an org-level secret.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Secret name.",
			},
			"value": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Decrypted secret value.",
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Current secret version.",
			},
		},
	}
}

func (d *SecretDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *SecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data secretDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := d.client.GetSecret(ctx, scope, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read secret", err.Error())
		return
	}

	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, secret.Name))
	data.Value = types.StringValue(secret.Value)
	data.Version = types.Int64Value(secret.Version)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
