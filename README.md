<h1 align="center">
 <img
  width="180"
  alt="Homer's donut"
  src="https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/homer/Homer-Operator.png">
    <br/>
    Homer-Operator
</h1>

The `homer-operator` is a Kubernetes operator designed to simplify the deployment and management of dynamic dashboards using the [bastienwirtz/homer](https://github.com/bastienwirtz/homer) container. This operator leverages Homer's extensible YAML configuration methodologies to automatically generate and update dashboards based on existing Ingress and API Gateway resources within the Kubernetes cluster.

## Features

- Automatic generation of dynamic dashboards based on existing Ingress and API Gateway resources.
- Simplified management of dashboards through Kubernetes custom resources.
- Utilizes Kubebuilder for seamless integration with Kubernetes.

## Prerequisites

Before running the `homer-operator`, ensure you have the following prerequisites installed:

- Kubernetes cluster (locally or externally accessible)
- `kubectl` configured to access the cluster
- Docker (if building the operator locally)

## Installation

### Running Locally

To run the `homer-operator` locally, follow these steps:

1. Clone the repository:

   ```bash
   git clone https://github.com/rajsinghtech/homer-operator.git
   ```

2. Change directory to the project:

   ```bash
   cd homer-operator
   ```

3. Build the operator:

   ```bash
   make install
   make build
   ```

4. Deploy the operator to your Kubernetes cluster:

   ```bash
   make deploy
   ```

### Running Externally

To run the `homer-operator` on an externally accessible Kubernetes cluster, you can use the pre-built Docker image available on Docker Hub:

```bash
kubectl apply -f https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/deploy/operator.yaml
```

This command will deploy the operator to your Kubernetes cluster using the pre-built Docker image.

## Usage

Once the `homer-operator` is running in your Kubernetes cluster, you can start creating dynamic dashboards by defining custom resources.

For example, you can create a dashboard for a specific application by defining a `Dashboard` custom resource:

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: dashboard-sample
spec:
  homerConfig:
    title: "Raj's Dashboard"
    subtitle: "Raj's Subtitle"
    # theme: default
    header: "false"
    footer: '<p>Homer-Operator</p>' 
    # columns: "3"
    logo: "https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/homer/Homer-Operator.png"
    defaults:
      layout: list
      colorTheme: auto
  configMap:
    name: "raj-config"
    key: "raj-key" 

```

This YAML manifest instructs the `homer-operator` to generate a dashboard titled "My Application Dashboard" with a description for monitoring an application labeled `app: my-application` within the namespace `my-namespace`.

## Contributing

We welcome contributions from the community. If you have any ideas, feature requests, or bug fixes, please feel free to open an issue or submit a pull request on [GitHub](https://github.com/rajsinghtech/homer-operator).
