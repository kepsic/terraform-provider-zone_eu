package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &DNSURLRecordResource{}
var _ resource.ResourceWithImportState = &DNSURLRecordResource{}

func NewDNSURLRecordResource() resource.Resource {
	return &DNSURLRecordResource{}
}

type DNSURLRecordResource struct {
	client *Client
}

type DNSURLRecordResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Zone          types.String `tfsdk:"zone"`
	Name          types.String `tfsdk:"name"`
	Destination   types.String `tfsdk:"destination"`
	RedirectType  types.Int64  `tfsdk:"redirect_type"`
	RecordID      types.String `tfsdk:"record_id"`
	ForceRecreate types.Bool   `tfsdk:"force_recreate"`
}

func (r *DNSURLRecordResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_url_record"
}

func (r *DNSURLRecordResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a DNS URL redirect record on Zone.EU.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource in format zone/record_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"zone": schema.StringAttribute{
				Description: "The DNS zone name (domain name, e.g., example.com).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The hostname to redirect from (FQDN, e.g., old.example.com).",
				Required:    true,
			},
			"destination": schema.StringAttribute{
				Description: "The URL to redirect to.",
				Required:    true,
			},
			"redirect_type": schema.Int64Attribute{
				Description: "The HTTP redirect status code: 301 (permanent) or 302 (temporary).",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"record_id": schema.StringAttribute{
				Description: "The ID of the record in Zone.EU.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"force_recreate": schema.BoolAttribute{
				Description: "If true, delete existing record with same name before creating. Default: false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *DNSURLRecordResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *DNSURLRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DNSURLRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If force_recreate is true, check for existing record and delete it
	if data.ForceRecreate.ValueBool() {
		existing, err := r.client.FindURLRecordByName(data.Zone.ValueString(), data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to check for existing URL record, got error: %s", err))
			return
		}
		if existing != nil {
			tflog.Info(ctx, "force_recreate: deleting existing URL record", map[string]interface{}{
				"zone":      data.Zone.ValueString(),
				"name":      data.Name.ValueString(),
				"record_id": existing.ID,
			})
			if err := r.client.DeleteURLRecord(data.Zone.ValueString(), existing.ID); err != nil {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete existing URL record for force_recreate, got error: %s", err))
				return
			}
		}
	}

	record := &DNSRecord{
		Name:         data.Name.ValueString(),
		Destination:  data.Destination.ValueString(),
		RedirectType: int(data.RedirectType.ValueInt64()),
	}

	created, err := r.client.CreateURLRecord(data.Zone.ValueString(), record)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create URL record, got error: %s", err))
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Zone.ValueString(), created.ID))
	data.RecordID = types.StringValue(created.ID)

	tflog.Trace(ctx, "created URL record")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DNSURLRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DNSURLRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, recordID, err := parseRecordID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse ID: %s", err))
		return
	}

	record, err := r.client.GetURLRecord(zone, recordID)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read URL record, got error: %s", err))
		return
	}

	data.Zone = types.StringValue(zone)
	data.Name = types.StringValue(record.Name)
	data.Destination = types.StringValue(record.Destination)
	data.RedirectType = types.Int64Value(int64(record.RedirectType))
	data.RecordID = types.StringValue(record.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DNSURLRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DNSURLRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, recordID, err := parseRecordID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse ID: %s", err))
		return
	}

	record := &DNSRecord{
		Name:         data.Name.ValueString(),
		Destination:  data.Destination.ValueString(),
		RedirectType: int(data.RedirectType.ValueInt64()),
	}

	_, err = r.client.UpdateURLRecord(zone, recordID, record)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update URL record, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "updated URL record")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DNSURLRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DNSURLRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, recordID, err := parseRecordID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse ID: %s", err))
		return
	}

	err = r.client.DeleteURLRecord(zone, recordID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete URL record, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted URL record")
}

func (r *DNSURLRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: zone/record_id. Got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("record_id"), parts[1])...)
}
