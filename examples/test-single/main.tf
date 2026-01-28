terraform {
  required_providers {
    zoneeu = {
      source  = "kepsic/zoneeu"
      version = "~> 1.0"
    }
  }
}

provider "zoneeu" {
  username = var.zone_eu_username
  api_key  = var.zone_eu_api_key
}

variable "zone_eu_username" {
  description = "Zone.eu username"
  type        = string
  sensitive   = true
}

variable "zone_eu_api_key" {
  description = "Zone.eu API key"
  type        = string
  sensitive   = true
}

# Simple data source to test provider connectivity
data "zoneeu_domain" "test" {
  name = "vermare.ee"
}

output "domain_info" {
  value = data.zoneeu_domain.test
}
