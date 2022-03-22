# app service account
resource "google_service_account" "app" {
  account_id   = var.app_name
  display_name = "${var.app_name} Service Account"
}

resource "google_project_iam_member" "cloudtrace" {
  project = var.project
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.app.email}"
}

resource "google_project_iam_member" "cloudprofiler" {
  project = var.project
  role    = "roles/cloudprofiler.agent"
  member  = "serviceAccount:${google_service_account.app.email}"
}

# app secret
data "google_secret_manager_secret" "slack-token" {
  secret_id = var.slack_token_secret
}

resource "google_secret_manager_secret_iam_member" "secret-access" {
  secret_id = data.google_secret_manager_secret.slack-token.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.app.email}"
}

# app invoker service account (e.g. Cloud Scheduler)
resource "google_service_account" "invoker" {
  account_id   = "${var.app_name}-invoker"
  display_name = "${var.app_name}-invoker Service Account"
}

resource "google_cloud_run_service_iam_policy" "invoker" {
  location    = google_cloud_run_service.app.location
  project     = google_cloud_run_service.app.project
  service     = google_cloud_run_service.app.name
  policy_data = data.google_iam_policy.invoker.policy_data
}

data "google_iam_policy" "invoker" {
  binding {
    role = "roles/run.invoker"
    members = [
      "serviceAccount:${google_service_account.invoker.email}",
    ]
  }
}
