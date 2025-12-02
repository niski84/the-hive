output "droplet_ip" {
  description = "Public IP address of the Hive server droplet"
  value       = digitalocean_droplet.hive_server.ipv4_address
}

output "droplet_id" {
  description = "ID of the DigitalOcean droplet"
  value       = digitalocean_droplet.hive_server.id
}

output "droplet_name" {
  description = "Name of the DigitalOcean droplet"
  value       = digitalocean_droplet.hive_server.name
}

