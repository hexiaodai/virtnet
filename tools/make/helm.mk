# This is a wrapper to manage helm chart
#
# All make targets related to helm are defined in this file.

.PHONY: helm.package
helm.package:
	@$(LOG_TARGET)
	@helm package charts --destination deploy --debug --version $(VERSION) --app-version $(VERSION)

.PHONY: helm.generate-template
helm.generate-template:
	@$(LOG_TARGET)
	@helm -n virtnet template \
		--set deployment.agent.image.repository=$(IMAGE_AGENT) \
		--set deployment.ctrl.image.repository=$(IMAGE_CTRL) \
		--set deployment.plugins.image.repository=$(IMAGE_PLUGINS) \
		deploy/$(HELM_NAME)-$(VERSION).tgz > deploy/virtnet.yaml
	@cp -r charts/crds deploy/

.PHONY: helm.push
helm.push:
	@$(LOG_TARGET)
	@helm push deploy/$(HELM_NAME)-$(VERSION).tgz $(OCI_REGISTRY)

##@ Helm

.PHONY: helm.release
helm.release: ## Package virtnet helm chart for release.
helm.release: helm.package helm.generate-template helm.push
