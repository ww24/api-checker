terraform {
  required_version = "~> 1.1.7"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 4.42.1"
    }
  }

  backend "gcs" {
    bucket = "chatbot-infra-terraform"
    prefix = "api-checker"
  }
}
