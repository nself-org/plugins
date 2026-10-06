mock_provider "hcloud" {
  mock_resource "hcloud_server" {
    defaults = {
      id = "12345"
    }
  }
  mock_resource "hcloud_firewall" {
    defaults = {
      id = "54321"
    }
  }
}
mock_provider "cloudflare" {}

variables {
  server_type = "cx21"
  location    = "fsn1"
  domain      = "myapp.com"
  ssh_keys    = ["my-key"]
}

run "valid_plan" {
  command = plan

  assert {
    condition     = hcloud_server.nself.server_type == "cx21"
    error_message = "server_type does not match"
  }
}
