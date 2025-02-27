variable "gcp_project" {
  description = "GCloud Project ID"
  type        = string
}

variable "gcp_region" {
  description = "GCloud Region"
  type        = string
  default     = "us-central1"
}

variable "system" {
  description = "The name of the Buz implementation. \n\nExample: buz"
  type        = string
  default     = "buz"
}

variable "env" {
  description = "The name of the Buz environment. \n\nExample: development/staging/production"
  type        = string
}

variable "buz_domain" {
  description = "The subdomain to map Buz to. \n\nExample: track.yourdomain.com"
  type        = string
}

variable "buz_version" {
  description = "The version of Buz to run. \n\nExample: v0.15.6"
  type        = string
  default     = "v0.15.6"
}

variable "buz_service_timeout_seconds" {
  description = "The service timeout in seconds"
  type        = number
  default     = 300 # Cloud Run default
}

variable "buz_service_container_concurrency" {
  description = "The service container concurrency"
  type        = number
  default     = 200
}

variable "buz_service_cpu_limit" {
  description = "The service cpu limit"
  type        = string
  default     = "1" # Cloud Run default
}

variable "buz_service_memory_limit" {
  description = "The service memory limit"
  type        = string
  default     = "512Mi" # Cloud Run default
}

variable "buz_service_container_port" {
  description = "The service container port"
  type        = number
  default     = 8080
}

variable "schema_bucket_location" {
  description = "The GCS schema bucket location"
  type        = string
  default     = "US"
}

variable "schema_bucket_name" {
  description = "The name of the GCS bucket for schemas. \n\nPLEASE NOTE! Buckets are globally unique so you may need to be creative."
  type        = string
}

variable "bigquery_location" {
  description = "The location to store data in BigQuery"
  type        = string
  default     = "US"
}

variable "bigquery_dataset_name" {
  description = "The name of the Bigquery dataset"
  type        = string
  default     = "buz"
}

variable "default_bigquery_table" {
  description = "The name of the default BigQuery table"
  type        = string
  default     = "invalid_events"
}

variable "deadletter_bigquery_table" {
  description = "The name of the deadletter BigQuery table"
  type        = string
  default     = "events"
}
