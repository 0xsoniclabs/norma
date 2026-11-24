# One short-lived VM that runs a single norma scenario, in a network of its own
# that nothing else in the project shares. Docker and the run's files live on a
# local NVMe SSD, which is fast and disappears with the VM. GCP deletes the VM
# and its disks once ttl_minutes pass, even if nobody runs `terraform destroy`.
# Embedded in and driven by `norma run-cloud`; see README.md.

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

variable "project" { type = string }
variable "zone" {
  type    = string
  default = "us-central1-a"
}
variable "run_id" { type = string }
variable "machine_type" {
  type    = string
  default = "n2-standard-16"
}
variable "disk_gb" {
  type        = number
  default     = 10
  description = "size of the boot disk, which holds only the OS"
}
variable "ttl_minutes" { type = number }

provider "google" {
  project = var.project
  zone    = var.zone
}

locals {
  name   = "norma-run-${var.run_id}"
  region = join("-", slice(split("-", var.zone), 0, 2))
}

resource "google_compute_network" "run" {
  name                    = local.name
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "run" {
  name          = local.name
  network       = google_compute_network.run.id
  region        = local.region
  ip_cidr_range = "10.0.0.0/24"
}

resource "google_compute_firewall" "iap_ssh" {
  name          = local.name
  network       = google_compute_network.run.id
  source_ranges = ["35.235.240.0/20"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

# Outbound access (apt, Docker Hub, Go modules) for a VM without external IP.
resource "google_compute_router" "run" {
  name    = local.name
  network = google_compute_network.run.id
  region  = local.region
}

resource "google_compute_router_nat" "run" {
  name                               = local.name
  router                             = google_compute_router.run.name
  region                             = local.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

resource "google_compute_instance" "vm" {
  name         = local.name
  machine_type = var.machine_type
  labels       = { norma-run = var.run_id }

  boot_disk {
    auto_delete = true
    initialize_params {
      image  = "debian-cloud/debian-13"
      size   = var.disk_gb
      type   = "pd-balanced"
      labels = { norma-run = var.run_id }
    }
  }

  scratch_disk {
    interface = "NVME"
  }

  # No access_config: no external IP. SSH goes through IAP.
  network_interface {
    subnetwork = google_compute_subnetwork.run.id
  }

  scheduling {
    provisioning_model          = "STANDARD"
    instance_termination_action = "DELETE"
    max_run_duration {
      seconds = var.ttl_minutes * 60
    }
  }

  depends_on = [google_compute_router_nat.run, google_compute_firewall.iap_ssh]

  metadata_startup_script = <<-EOT
    #!/bin/bash
    set -euo pipefail
    [ -f /ready ] && exit 0
    ssd=/dev/disk/by-id/google-local-nvme-ssd-0
    mkfs.ext4 -F -q $ssd
    mkdir -p /mnt/ssd && mount -o discard,noatime $ssd /mnt/ssd
    for dir in /var/lib/docker /var/lib/containerd /opt/norma; do
      mkdir -p /mnt/ssd$dir $dir && mount --bind /mnt/ssd$dir $dir
    done
    curl -fsSL https://get.docker.com | sh
    touch /ready
  EOT
}

output "instance_name" {
  value = google_compute_instance.vm.name
}
