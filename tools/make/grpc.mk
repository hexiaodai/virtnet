##@ grpc

API_VERSIONS = v1alpha1 v1alpha2

.PHONY: grpc.generate
grpc.generate: ## Generated client and server code.
	@$(LOG_TARGET)
	@for version in $(API_VERSIONS); do \
		pkg_dir=$(ROOT_DIR)/api/$$version; \
		if ! [ -d "$$pkg_dir" ]; then \
			continue; \
		fi; \
		for full in $$pkg_dir/*; do \
			if ! [ -d "$$full" ]; then \
				continue; \
			fi; \
			pkg=$$(realpath --relative-to=$(ROOT_DIR)/api/ $$full); \
			protoc --go_out=$(ROOT_DIR)/api \
			--go-grpc_out=$(ROOT_DIR)/api \
			--go-grpc_opt require_unimplemented_servers=false \
		    --validate_out="lang=go:$(ROOT_DIR)/api" \
			--proto_path=$(ROOT_DIR)/api $$full/*.proto; \
		done \
	done
