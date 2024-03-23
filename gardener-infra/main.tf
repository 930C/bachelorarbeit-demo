variable "config_path" {
  type        = string
  default     = "~/.kube/config"
  description = "Kubeconfig path"
}

variable "username" {
  type = string
}

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.0"
    }
  }
}

provider "kubernetes" {
  config_path = var.config_path
}

resource "kubernetes_namespace" "monitoring" {
  metadata {
    name = "monitoring"
  }
}

provider "helm" {
  kubernetes {
    config_path = var.config_path
  }
}

resource "helm_release" "kube_prometheus_stack" {
  depends_on = [kubernetes_namespace.monitoring]
  name       = "prometheus-stack"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  version    = "57.0.3"

  set {
    name  = "namespaceOverride"
    value = "monitoring"
  }

  set {
    name  = "grafana.namespaceOverride"
    value = "monitoring"
  }

  set {
    name  = "prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues"
    value = false
  }
}