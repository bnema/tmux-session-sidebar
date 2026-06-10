PLUGIN_NAME ?= tmux-session-sidebar
PLUGIN_REPO ?= bnema/$(PLUGIN_NAME)
TPM_DIR ?= $(HOME)/.tmux/plugins
TARGET_DIR ?= $(TPM_DIR)/$(PLUGIN_NAME)

.PHONY: install uninstall mocks test-go test-runtime-bootstrap build-runtime dev-runtime prod-runtime restart-runtime update-runtime

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
	@bash scripts/update-runtime_test.sh
	@bash scripts/ensure-runtime_test.sh
	@bash scripts/no-git-update-hook_test.sh
	@bash scripts/remove-git-update-hook_test.sh
	@bash scripts/daemon-control_test.sh
	@bash scripts/restart-runtime_test.sh

build-runtime:
	@runtime_bin="$$(TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1 bash scripts/update-runtime.sh --ensure)"; status=$$?; \
		if [ $$status -ne 0 ] || [ -z "$$runtime_bin" ]; then \
			echo "Failed to update tmux plugin runtime" >&2; \
			exit 1; \
		fi; \
		echo "Updated tmux plugin runtime -> $$runtime_bin"

dev-runtime:
	@runtime_bin="$$(TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1 bash scripts/update-runtime.sh --ensure)"; status=$$?; \
		if [ $$status -ne 0 ] || [ -z "$$runtime_bin" ]; then \
			echo "Failed to update tmux plugin dev runtime" >&2; \
			exit 1; \
		fi; \
		touch .bin/.dev-runtime; \
		pkill -f "$$runtime_bin daemon serve-ui" 2>/dev/null || true; \
		pkill -f "$$runtime_bin daemon bootstrap" 2>/dev/null || true; \
		pkill -f "$$runtime_bin daemon serve" 2>/dev/null || true; \
		if command -v tmux >/dev/null 2>&1; then \
			tmux kill-session -t __tmux-session-sidebar 2>/dev/null || true; \
		fi; \
		echo "Updated dev runtime -> $$runtime_bin"; \
		echo "Dev runtime marker written to .bin/.dev-runtime"; \
		echo "Sidebar closed; reopen it manually to use the dev binary."

prod-runtime:
	@rm -f .bin/.dev-runtime
	@bash scripts/update-runtime.sh

restart-runtime:
	@TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1 bash scripts/update-runtime.sh

update-runtime:
	@rm -f .bin/.dev-runtime
	@bash scripts/update-runtime.sh

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
