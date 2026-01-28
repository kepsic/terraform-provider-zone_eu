# Terraform Provider for Zone.EU

This is a Terraform provider for managing DNS records on [Zone.EU](https://www.zone.ee) hosting platform via their [API v2](https://api.zone.eu/v2).

## Features

### Implemented (DNS Management)

- **DNS A Record** - IPv4 address records
- **DNS AAAA Record** - IPv6 address records
- **DNS CNAME Record** - Canonical name (alias) records
- **DNS MX Record** - Mail exchange records
- **DNS TXT Record** - Text records (SPF, DKIM, etc.)
- **DNS NS Record** - Nameserver records
- **DNS SRV Record** - Service locator records
- **DNS CAA Record** - Certificate Authority Authorization records
- **DNS TLSA Record** - DANE/TLS authentication records
- **DNS SSHFP Record** - SSH fingerprint records
- **DNS URL Record** - URL redirect records (Zone.EU specific)

### Data Sources

- **DNS Zone** - Read DNS zone information

### Not Yet Implemented

The Zone.EU API supports many other services that are not yet implemented in this provider:

- **Domain Management** - Domain registration, renewal, nameserver management
- **Webhosting (vserver)** - Virtual server management
- **E-mail** - Email account management
- **MySQL** - Database management
- **SSL Certificates** - SSL/TLS certificate management
- **Crontab** - Scheduled task management
- **Redis** - Redis database management
- **PM2** - Node.js process management
- **SSH** - SSH key and whitelist management
- **Port Forward** - Port forwarding configuration
- **Cloudserver VPS** - Cloud VPS management

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 0.12
- [Go](https://golang.org/doc/install) >= 1.21 (for building from source)
- Zone.EU account with API access enabled

## Installation

### Building from source

```bash
git clone https://github.com/terraform-providers/terraform-provider-zone_eu.git
cd terraform-provider-zone_eu
go build -o terraform-provider-zone_eu
```

### Installing locally

Copy the built binary to your Terraform plugins directory:

```bash
# Linux/macOS
mkdir -p ~/.terraform.d/plugins
cp terraform-provider-zone_eu ~/.terraform.d/plugins/

# Or for Terraform 0.13+
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/zone/zone_eu/1.0.0/darwin_amd64/
cp terraform-provider-zone_eu ~/.terraform.d/plugins/registry.terraform.io/zone/zone_eu/1.0.0/darwin_amd64/
```

## Configuration

### Provider Configuration

```hcl
provider "zone" {
  username = "your-zoneid-username"
  api_key  = "your-zoneid-api-key"
}
```

You can also use environment variables:

```bash
export ZONE_EU_USERNAME="your-zoneid-username"
export ZONE_EU_API_KEY="your-zoneid-api-key"
```

### Authentication

The provider uses HTTP Basic Auth. You need:
1. Your ZoneID username
2. An API key generated in your ZoneID account management

## Usage Examples

### A Record

```hcl
resource "zone_dns_a_record" "www" {
  zone        = "example.com"
  name        = "www.example.com"
  destination = "192.168.1.1"
}
```

### AAAA Record (IPv6)

```hcl
resource "zone_dns_aaaa_record" "www" {
  zone        = "example.com"
  name        = "www.example.com"
  destination = "2001:db8::1"
}
```

### CNAME Record

```hcl
resource "zone_dns_cname_record" "blog" {
  zone        = "example.com"
  name        = "blog.example.com"
  destination = "bloghost.example.net"
}
```

### MX Record

```hcl
resource "zone_dns_mx_record" "mail" {
  zone        = "example.com"
  name        = "example.com"
  destination = "mail.example.com"
  priority    = 10
}
```

### TXT Record (SPF)

```hcl
resource "zone_dns_txt_record" "spf" {
  zone        = "example.com"
  name        = "example.com"
  destination = "v=spf1 include:_spf.google.com ~all"
}
```

### NS Record

```hcl
resource "zone_dns_ns_record" "subdomain" {
  zone        = "example.com"
  name        = "subdomain.example.com"
  destination = "ns1.otherdns.com"
}
```

### SRV Record

```hcl
resource "zone_dns_srv_record" "sip" {
  zone        = "example.com"
  name        = "_sip._tcp.example.com"
  destination = "sipserver.example.com"
  priority    = 10
  weight      = 5
  port        = 5060
}
```

### CAA Record

```hcl
resource "zone_dns_caa_record" "letsencrypt" {
  zone        = "example.com"
  name        = "example.com"
  destination = "letsencrypt.org"
  flag        = 0
  tag         = "issue"
}
```

### TLSA Record (DANE)

```hcl
resource "zone_dns_tlsa_record" "https" {
  zone              = "example.com"
  name              = "_443._tcp.example.com"
  destination       = "abc123def456..."
  certificate_usage = 3
  selector          = 1
  matching_type     = 1
}
```

### SSHFP Record

```hcl
resource "zone_dns_sshfp_record" "server" {
  zone        = "example.com"
  name        = "server.example.com"
  destination = "abc123def456..."
  algorithm   = 4  # Ed25519
  type        = 2  # SHA-256
}
```

### URL Redirect Record

```hcl
resource "zone_dns_url_record" "redirect" {
  zone          = "example.com"
  name          = "old.example.com"
  destination   = "https://new.example.com/page"
  redirect_type = 301
}
```

### Data Source: DNS Zone

```hcl
data "zone_dns_zone" "main" {
  name = "example.com"
}

output "zone_active" {
  value = data.zone_dns_zone.main.active
}
```

## Importing Existing Resources

All resources support importing using the format `zone/record_id`:

```bash
terraform import zone_dns_a_record.www example.com/123
```

## API Rate Limits

The Zone.EU API has a rate limit of 60 requests per minute per IP address. The provider does not currently implement rate limiting, so be cautious with large configurations.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (version 1.21+ is *required*).

To compile the provider, run:

```sh
go build -o terraform-provider-zone_eu
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This provider is distributed under the [Mozilla Public License 2.0](LICENSE).
