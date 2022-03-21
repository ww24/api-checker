resource "google_storage_bucket" "backend" {
  project                     = var.project
  name                        = "chatbot-infra-terraform"
  location                    = "ASIA-NORTHEAST1"
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true

  labels = {
    service = "terraform"
  }
}
