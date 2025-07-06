#!/bin/bash

# Script to sync CRD to Helm chart template
# This script wraps the CRD with Helm templating

set -euo pipefail

# Check arguments
if [ $# -ne 1 ]; then
    echo "Usage: $0 <crd-file>" >&2
    echo "  Converts a Kubernetes CRD file to a Helm template" >&2
    exit 1
fi

CRD_FILE="$1"

# Validate input file
if [ ! -f "$CRD_FILE" ]; then
    echo "Error: CRD file '$CRD_FILE' not found" >&2
    exit 1
fi

# Check if file is readable
if [ ! -r "$CRD_FILE" ]; then
    echo "Error: CRD file '$CRD_FILE' is not readable" >&2
    exit 1
fi

# Validate that the file contains CRD content
if ! grep -q "kind: CustomResourceDefinition" "$CRD_FILE"; then
    echo "Error: File '$CRD_FILE' does not appear to be a CRD (missing 'kind: CustomResourceDefinition')" >&2
    exit 1
fi

cat <<EOF
{{- if .Values.crd.create }}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: dashboards.homer.rajsingh.info
  labels:
    {{- include "homer-operator.labels" . | nindent 4 }}
  annotations:
    controller-gen.kubebuilder.io/version: v0.14.0
    {{- with .Values.crd.annotations }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
EOF

# Extract spec section from the CRD (skip first 8 lines of metadata, remove last line)
# Check if the file has enough lines
LINE_COUNT=$(wc -l < "$CRD_FILE")
if [ "$LINE_COUNT" -lt 9 ]; then
    echo "Error: CRD file '$CRD_FILE' appears to be too short (less than 9 lines)" >&2
    exit 1
fi

# Extract and validate the spec section
SPEC_CONTENT=$(tail -n +9 "$CRD_FILE" | sed '$d')
if [ -z "$SPEC_CONTENT" ]; then
    echo "Error: No spec content found in CRD file '$CRD_FILE'" >&2
    exit 1
fi

echo "$SPEC_CONTENT"

echo "{{- end }}"