# Fix Zone.eu Terraform Provider: Handle zone_conflict Errors

## Problem

When Terraform tries to create a DNS record that already exists in Zone.eu, the API returns a 422 error with `{"name":"zone_conflict"}`. Currently the provider just errors out, leaving Terraform state out of sync with reality.

```
Error: Client Error

Unable to create CNAME record, got error: API error (status 422):
{"name":"zone_conflict"} (X-Status-Message: Puudulik sisend)
```

## Required Fix

In each DNS resource's Create function, after the create API call fails with "zone_conflict", the provider should:

1. Find the existing record by name using the existing `FindXXXRecordByName()` client method
2. Adopt that record into Terraform state by setting the ID and RecordID
3. Return successfully (not error)

This makes Terraform idempotent - if a record exists, it gets adopted instead of erroring.

## Files to Modify

Apply this pattern to all DNS record resources:

- `internal/provider/resource_dns_a_record.go`
- `internal/provider/resource_dns_aaaa_record.go`
- `internal/provider/resource_dns_cname_record.go`
- `internal/provider/resource_dns_txt_record.go`
- `internal/provider/resource_dns_mx_record.go`
- `internal/provider/resource_dns_ns_record.go`
- `internal/provider/resource_dns_srv_record.go`
- `internal/provider/resource_dns_caa_record.go`
- `internal/provider/resource_dns_sshfp_record.go`
- `internal/provider/resource_dns_tlsa_record.go`
- `internal/provider/resource_dns_url_record.go`

## Example Fix for CNAME

In the `Create` function, change this:

```go
created, err := r.client.CreateCNAMERecord(data.Zone.ValueString(), record)
if err != nil {
    resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create CNAME record, got error: %s", err))
    return
}
```

To this:

```go
created, err := r.client.CreateCNAMERecord(data.Zone.ValueString(), record)
if err != nil {
    // Handle zone_conflict by adopting existing record into state
    if strings.Contains(err.Error(), "zone_conflict") {
        tflog.Info(ctx, "Record already exists (zone_conflict), adopting into state", map[string]interface{}{
            "zone": data.Zone.ValueString(),
            "name": data.Name.ValueString(),
        })
        existing, findErr := r.client.FindCNAMERecordByName(data.Zone.ValueString(), data.Name.ValueString())
        if findErr == nil && existing != nil {
            data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Zone.ValueString(), existing.ID))
            data.RecordID = types.StringValue(existing.ID)
            resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
            return
        }
        // If we couldn't find/adopt, fall through to error
    }
    resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create CNAME record, got error: %s", err))
    return
}
```

## Pattern for Each Record Type

| Record Type | Create Method | Find Method |
|-------------|---------------|-------------|
| A | `CreateARecord` | `FindARecordByName` |
| AAAA | `CreateAAAARecord` | `FindAAAARecordByName` |
| CNAME | `CreateCNAMERecord` | `FindCNAMERecordByName` |
| TXT | `CreateTXTRecord` | `FindTXTRecordByName` |
| MX | `CreateMXRecord` | `FindMXRecordByName` |
| NS | `CreateNSRecord` | `FindNSRecordByName` |
| SRV | `CreateSRVRecord` | `FindSRVRecordByName` |
| CAA | `CreateCAARecord` | `FindCAARecordByName` |
| SSHFP | `CreateSSHFPRecord` | `FindSSHFPRecordByName` |
| TLSA | `CreateTLSARecord` | `FindTLSARecordByName` |
| URL | `CreateURLRecord` | `FindURLRecordByName` |

## Important Notes

1. Each record type has its own Create function and FindByName method
2. The `strings` package is already imported in all resource files
3. The `tflog` package is already imported in all resource files
4. This fix makes Terraform idempotent - running `terraform apply` multiple times won't fail if records already exist
5. The existing record gets "adopted" into Terraform state, allowing future updates/deletes to work correctly

## Testing

After making changes:

```bash
# Build the provider
go build -o terraform-provider-zoneeu

# Install locally
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/kepsic/zoneeu/99.0.0/darwin_arm64
cp terraform-provider-zoneeu ~/.terraform.d/plugins/registry.terraform.io/kepsic/zoneeu/99.0.0/darwin_arm64/terraform-provider-zoneeu_v99.0.0

# Test with terraform
cd /path/to/terraform/config
terraform init -upgrade
terraform apply
```

## Expected Behavior After Fix

When a record already exists:

```
module.tenant["ee"].zoneeu_dns_cname_record.app["app.vermare.ee"]: Creating...
2026-01-28T17:00:00.000Z [INFO] Record already exists (zone_conflict), adopting into state
module.tenant["ee"].zoneeu_dns_cname_record.app["app.vermare.ee"]: Creation complete after 1s

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
```
