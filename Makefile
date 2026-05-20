PLUGIN_NAME ?= tmux-session-sidebar
PLUGIN_REPO ?= bnema/$(PLUGIN_NAME)
TPM_DIR ?= $(HOME)/.tmux/plugins
TARGET_DIR ?= $(TPM_DIR)/$(PLUGIN_NAME)
GO_BIN_DIR ?= $(shell go env GOBIN 2>/dev/null || true)
ifeq ($(GO_BIN_DIR),)
GO_BIN_DIR := $(shell go env GOPATH)/bin
endif
GO_BIN_PATH ?= $(GO_BIN_DIR)/tmux-session-sidebar

.PHONY: install uninstall mocks test-go test-runtime-bootstrap go-install

install:
	@mkdir -p "$(TPM_DIR)"
	@ln -sfn "$(CURDIR)" "$(TARGET_DIR)"
	@echo "Installed $(PLUGIN_NAME) -> $(TARGET_DIR)"
	@echo "If needed, add this to ~/.tmux.conf:"
	@echo "  set -g @plugin '$(PLUGIN_REPO)'"
	@echo "  run '~/.tmux/plugins/tpm/tpm'"
	@echo "Then reload tmux or press prefix + I."

mocks:
	@mockery

test-go:
	@go test ./...

test-runtime-bootstrap:
	@bash scripts/ensure-runtime_test.sh

go-install:
	@mkdir -p "$(GO_BIN_DIR)"
	@go install ./cmd/tmux-session-sidebar
	@runtime_bin="$$(bash scripts/ensure-runtime.sh)"; status=$$?; \
		if [ $$status -ne 0 ] || [ -z "$$runtime_bin" ]; then \
			echo "Failed to update tmux plugin runtime" >&2; \
			exit 1; \
		fi; \
		echo "Installed Go runtime -> $(GO_BIN_PATH)"; \
		echo "Updated tmux plugin runtime -> $$runtime_bin"

uninstall:
	@[ -n "$(TARGET_DIR)" ] || { echo "Error: TARGET_DIR is empty" >&2; exit 1; }
	@[ "$(TARGET_DIR)" != "/" ] || { echo "Error: refusing to remove /" >&2; exit 1; }
	@[ "$(TARGET_DIR)" != "." ] || { echo "Error: refusing to remove ." >&2; exit 1; }
	@[ "$(TARGET_DIR)" != ".." ] || { echo "Error: refusing to remove .." >&2; exit 1; }
	@resolved_tpm_dir=$$(realpath -m "$(TPM_DIR)" 2>/dev/null || readlink -f "$(TPM_DIR)" 2>/dev/null || printf '%s' "$(TPM_DIR)"); \
	target_parent_dir=$$(dirname -- "$(TARGET_DIR)"); \
	resolved_target_parent=$$(realpath -m "$$target_parent_dir" 2>/dev/null || readlink -f "$$target_parent_dir" 2>/dev/null || printf '%s' "$$target_parent_dir"); \
	case "$$resolved_target_parent" in "$$resolved_tpm_dir"|"$$resolved_tpm_dir"/*) ;; *) echo "Error: refusing to remove unsafe target $(TARGET_DIR)" >&2; exit 1 ;; esac
	@if [ -e "$(TARGET_DIR)" ] && [ ! -L "$(TARGET_DIR)" ]; then \
		echo "Error: refusing to remove non-symlink target $(TARGET_DIR)" >&2; \
		exit 1; \
	fi
	@find "$(TARGET_DIR)" -maxdepth 0 -type l -exec rm -f -- {} \; 2>/dev/null || true
	@echo "Removed $(TARGET_DIR)"
