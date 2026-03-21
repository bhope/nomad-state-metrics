job "nomad-state-metrics" {
  # Use "system" to run one instance per client node.
  # Change to "service" with count = 1 for a single centralized exporter.
  type = "system"

  group "exporter" {
    network {
      port "metrics" {
        static = 9441
      }
      port "telemetry" {
        static = 9442
      }
    }

    task "nomad-state-metrics" {
      driver = "docker"

      config {
        image = "ghcr.io/bhope/nomad-state-metrics:latest"
        ports = ["metrics", "telemetry"]

        args = [
          "-nomad-address", "http://${attr.unique.network.ip-address}:4646",
          "-port", "9441",
          "-telemetry-port", "9442",
          "-poll-interval", "30s",
          "-log-level", "info",
        ]
      }

      # Allow the exporter to reach the local Nomad agent.
      env {
        NOMAD_ADDR = "http://${attr.unique.network.ip-address}:4646"
      }

      resources {
        cpu    = 50
        memory = 64
      }

      service {
        name = "nomad-state-metrics"
        port = "metrics"
        tags = ["prometheus"]

        check {
          type     = "http"
          path     = "/healthz"
          port     = "telemetry"
          interval = "15s"
          timeout  = "3s"
        }
      }
    }
  }
}
