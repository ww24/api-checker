resource "google_cloud_scheduler_job" "trigger" {
  count = var.enabled ? 1 : 0

  name             = "${var.app_name}-trigger"
  description      = "${var.app_name} trigger"
  schedule         = "0 * * * *"
  time_zone        = "Asia/Tokyo"
  attempt_deadline = "120s"

  http_target {
    http_method = "POST"
    uri         = "${google_cloud_run_service.app[0].status[0].url}/"
    headers = {
      content-type = "application/json"
    }
    body = var.request_body

    oidc_token {
      service_account_email = google_service_account.invoker.email
      audience              = google_cloud_run_service.app[0].status[0].url
    }
  }

  retry_config {
    retry_count          = 5
    min_backoff_duration = "1s"
    max_backoff_duration = "10s"
    max_doublings        = 2
  }
}
