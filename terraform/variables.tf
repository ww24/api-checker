variable "location" {
  type    = string
  default = "asia-northeast1"
}

variable "project" {
  type = string
}

variable "app_name" {
  type    = string
  default = "api-checker"
}

variable "gar_repository" {
  type    = string
  default = "ww24"
}

variable "image_name" {
  type    = string
  default = "github.com/ww24/api-checker"
}

variable "image_tag" {
  type    = string
  default = "latest"
}

variable "slack_channel" {
  type    = string
  default = "C038GV0J8QY"
}

variable "slack_token_secret" {
  type    = string
  default = "api-checker-slack-token"
}

variable "request_body" {
  type        = string
  description = "base64 encoded json payload to submit from Cloud Scheduler Job"
}
