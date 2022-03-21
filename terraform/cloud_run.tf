data "google_cloud_run_service" "app" {
  name     = var.app_name
  location = var.location
}

locals {
  current_image = data.google_cloud_run_service.app.template != null ? data.google_cloud_run_service.app.template[0].spec[0].containers[0].image : null
  new_image     = "${var.location}-docker.pkg.dev/${var.project}/${var.gar_repository}/${var.image_name}:${var.image_tag}"
  image         = (local.current_image != null && var.image_tag == "latest") ? local.current_image : local.new_image
  image_tag     = split(":", local.image)[1]
}

resource "google_cloud_run_service" "app" {
  name     = var.app_name
  location = var.location
  project  = var.project

  template {
    spec {
      service_account_name = google_service_account.app.email

      timeout_seconds = 30
      # set 1 because https://cloud.google.com/run/docs/configuring/cpu#setting
      container_concurrency = 1
      containers {
        image = local.image

        resources {
          limits = {
            cpu    = "500m"
            memory = "128Mi"
          }
        }

        env {
          name  = "SLACK_CHANNEL"
          value = var.slack_channel
        }

        env {
          name = "SLACK_TOKEN"
          value_from {
            secret_key_ref {
              name = data.google_secret_manager_secret.slack-token.secret_id
              key  = "latest"
            }
          }
        }
      }
    }

    metadata {
      # The revision name must be prefixed by the name of the enclosing Service or Configuration with a trailing -
      # Resource name must use only lowercase letters, numbers and '-'. Must begin with a letter and cannot end with a '-'. Maximum length is 63 characters.
      name = local.image_tag == "latest" ? null : "${var.app_name}-v${replace(local.image_tag, ".", "-")}"

      annotations = {
        "autoscaling.knative.dev/maxScale" = "1"
        "run.googleapis.com/launch-stage"  = "BETA"
      }

      labels = {
        service = var.app_name
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }

  autogenerate_revision_name = local.image_tag == "latest"
}
