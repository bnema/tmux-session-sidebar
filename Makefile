PLUGIN_NAME ?= tmux-session-sidebar
TPM_DIR ?= $(HOME)/.tmux/plugins
TARGET_DIR ?= $(TPM_DIR)/$(PLUGIN_NAME)

.PHONY: install uninstall

install:
	@mkdir -p "$(TPM_DIR)"
	@ln -sfn "$(CURDIR)" "$(TARGET_DIR)"
	@echo "Installed $(PLUGIN_NAME) -> $(TARGET_DIR)"
	@echo "If needed, add this to ~/.tmux.conf:"
	@echo "  set -g @plugin 'brice/$(PLUGIN_NAME)'"
	@echo "  run '~/.tmux/plugins/tpm/tpm'"
	@echo "Then reload tmux or press prefix + I."

uninstall:
	@rm -rf "$(TARGET_DIR)"
	@echo "Removed $(TARGET_DIR)"
