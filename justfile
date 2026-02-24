# Justfile for Snowplane
# https://github.com/casey/just

# Default recipe
default: build

# Build the controller manager binary
build:
    go build -o bin/manager ./cmd/manager

# Run unit tests with coverage
test:
    go test -race -coverprofile=coverage.out -covermode=atomic ./...
    @go tool cover -func=coverage.out | tail -1

# Run tests in short mode
test-short:
    go test -short -race ./...

# Run envtest integration tests (requires setup-envtest)
test-integration:
    KUBEBUILDER_ASSETS="$(setup-envtest use -p path)" go test -tags integration -v -timeout 180s -count=1 ./test/integration/

# Run E2E tests (self-contained: spins up a k3s testcontainer automatically)
test-e2e:
    go test -tags e2e -v -timeout 20m -count=1 ./test/e2e/

# Bootstrap kind cluster for manual / interactive E2E testing
e2e-setup-kind:
    ./hack/setup-e2e-kind.sh

# Tear down kind cluster after manual E2E testing
e2e-teardown-kind:
    ./hack/teardown-e2e-kind.sh

# Run linter
lint:
    golangci-lint run ./...

# Format code
fmt:
    go fmt ./...
    goimports -w -local github.com/hupe1980/snowplane .

# Vet code
vet:
    go vet ./...

# Build Docker image
docker-build img='ghcr.io/hupe1980/snowplane:dev':
    docker build -t {{img}} .

# Run controller locally
run:
    go run ./cmd/manager

# Install CRDs to cluster
install:
    kubectl apply -f config/crd/bases/

# Uninstall CRDs from cluster
uninstall:
    kubectl delete -f config/crd/bases/

# Run all checks (CI equivalent)
ci: lint vet test build

# Generate deepcopy methods and CRD manifests
generate:
    controller-gen object paths="./api/v1alpha1"
    controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

# Generate CRD manifests only
manifests:
    controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

# Sync generated CRDs into the Helm chart
sync-crds: manifests
    cp config/crd/bases/*.yaml charts/snowplane/crds/

# Verify Helm chart CRDs match generated CRDs (CI check)
verify-crds:
    @diff -rq config/crd/bases/ charts/snowplane/crds/ || (echo "ERROR: CRD drift detected. Run 'just sync-crds' to fix." && exit 1)

# Lint the Helm chart
helm-lint:
    helm lint charts/snowplane/

# Render Helm chart templates (dry-run validation)
helm-template:
    helm template snowplane charts/snowplane/

# Verify Helm chart lints and templates render cleanly (CI check)
verify-helm: helm-lint
    @helm template snowplane charts/snowplane/ > /dev/null

# Clean build artifacts
clean:
    rm -rf bin/ coverage.out
