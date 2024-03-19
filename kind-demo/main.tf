terraform {
  required_providers {
    kind = {
      source = "tehcyx/kind"
      version = "0.4.0"
    }
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

provider "kind" {}

resource "kind_cluster" "default" {
    name = "kindcluster"
    wait_for_ready = true

    kind_config {
        kind = "Cluster"
        api_version = "kind.x-k8s.io/v1alpha4"

        node {
            role = "control-plane"
        }

        node {
            role = "worker"
        }
    }
}

provider "kubernetes" {
  config_path = kind_cluster.default.kubeconfig_path
}

resource "kubernetes_namespace" "monitoring" {
  depends_on = [kind_cluster.default]
  metadata {
    name = "monitoring"
  }
}

provider "helm" {
  kubernetes {
    config_path = kind_cluster.default.kubeconfig_path
  }
}

resource "helm_release" "kube_prometheus_stack" {
  depends_on = [ kubernetes_namespace.monitoring ]
  name       = "prometheus-stack"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  version    = "57.0.3"

  set {
    name = "namespaceOverride"
    value = "monitoring"
  }

  set {
    name = "grafana.namespaceOverride"
    value = "monitoring"
  }

  set {
    name = "prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues"
    value = false
  }
}