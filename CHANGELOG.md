# Changelog

All notable changes to the Zone.EU Terraform Provider will be documented in this file.

## [Unreleased]

### Added
- Initial release of Zone.EU Terraform Provider
- DNS A record management (`zone_dns_a_record`)
- DNS AAAA record management (`zone_dns_aaaa_record`)
- DNS CNAME record management (`zone_dns_cname_record`)
- DNS MX record management (`zone_dns_mx_record`)
- DNS TXT record management (`zone_dns_txt_record`)
- DNS NS record management (`zone_dns_ns_record`)
- DNS SRV record management (`zone_dns_srv_record`)
- DNS CAA record management (`zone_dns_caa_record`)
- DNS TLSA record management (`zone_dns_tlsa_record`)
- DNS SSHFP record management (`zone_dns_sshfp_record`)
- DNS URL redirect record management (`zone_dns_url_record`)
- DNS Zone data source (`zone_dns_zone`)
- Support for import of existing records
- Authentication via environment variables (`ZONE_EU_USERNAME`, `ZONE_EU_API_KEY`)

### Notes
- Built with Terraform Plugin Framework v1.5.0
- Requires Go 1.21+
- API documentation: https://api.zone.eu/v2
