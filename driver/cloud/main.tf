# Short-lived VMs for one norma run: one for the driver and one for each node,
# in a network of their own that nothing else in the project shares. The
# driver reaches each node VM's Docker daemon over the private network. Docker
# and the run's files live on local NVMe SSDs, which disappear with the VMs.
# GCP deletes the VMs and their disks once ttl_minutes pass, even if nobody
# runs `terraform destroy`.
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
variable "driver_machine_type" {
  type    = string
  default = "n2-standard-4"
}
variable "node_machine_types" {
  type        = list(string)
  description = "the machine type of each node VM"
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
  name        = "norma-run-${var.run_id}"
  region      = join("-", slice(split("-", var.zone), 0, 2))
  driver_tag  = "${local.name}-driver"
  node_tag    = "${local.name}-node"
  subnet_cidr = "10.0.0.0/20"

  # Puts Docker and /opt/norma on the local SSD, installs Docker and starts
  # it with the extra arguments given, then marks the VM ready.
  startup_script = <<-EOT
    #!/bin/bash
    set -euo pipefail
    [ -f /ready ] && exit 0
    ssd=/dev/disk/by-id/google-local-nvme-ssd-0
    mkfs.ext4 -F -q $ssd
    mkdir -p /mnt/ssd && mount -o discard,noatime $ssd /mnt/ssd
    for dir in /var/lib/docker /var/lib/containerd /opt/norma; do
      mkdir -p /mnt/ssd$dir $dir && mount --bind /mnt/ssd$dir $dir
    done
    mkdir -p /etc/systemd/system/docker.service.d
    cat > /etc/systemd/system/docker.service.d/hosts.conf <<'CONF'
    [Service]
    ExecStart=
    ExecStart=/usr/bin/dockerd -H fd:// --containerd=/run/containerd/containerd.sock DOCKERD_ARGS
    CONF
    curl -fsSL https://get.docker.com | sh
    systemctl daemon-reload && systemctl restart docker
    touch /ready
  EOT
}

resource "google_compute_network" "run" {
  name                    = local.name
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "run" {
  name          = local.name
  network       = google_compute_network.run.id
  region        = local.region
  ip_cidr_range = local.subnet_cidr
}

resource "google_compute_firewall" "iap_ssh" {
  name          = local.name
  network       = google_compute_network.run.id
  source_ranges = ["35.235.240.0/20"]
  target_tags   = [local.driver_tag]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

# Nodes peer with each other, and the driver reaches their RPC and metrics.
resource "google_compute_firewall" "nodes" {
  name          = "${local.name}-nodes"
  network       = google_compute_network.run.id
  source_ranges = [local.subnet_cidr]
  target_tags   = [local.node_tag]
  allow {
    protocol = "tcp"
    ports    = ["5050", "18545", "18546", "6060"]
  }
  allow {
    protocol = "udp"
    ports    = ["5050"]
  }
}

# Only the driver may use the nodes' Docker daemons, which have no TLS.
resource "google_compute_firewall" "docker" {
  name        = "${local.name}-docker"
  network     = google_compute_network.run.id
  source_tags = [local.driver_tag]
  target_tags = [local.node_tag]
  allow {
    protocol = "tcp"
    ports    = ["2375"]
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
  machine_type = var.driver_machine_type
  labels       = { norma-run = var.run_id }
  tags         = [local.driver_tag]

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

  metadata_startup_script = replace(local.startup_script, "DOCKERD_ARGS", "")
}

resource "google_compute_instance" "node" {
  count        = length(var.node_machine_types)
  name         = "${local.name}-node-${count.index}"
  machine_type = var.node_machine_types[count.index]
  labels       = { norma-run = var.run_id }
  tags         = [local.node_tag]

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

  depends_on = [google_compute_router_nat.run, google_compute_firewall.docker]

  metadata_startup_script = replace(local.startup_script, "DOCKERD_ARGS", "-H tcp://0.0.0.0:2375")
}

output "instance_name" {
  value = google_compute_instance.vm.name
}

output "node_ips" {
  value = google_compute_instance.node[*].network_interface[0].network_ip
}
