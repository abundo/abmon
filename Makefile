# Build directory
BUILD_DIR := build

# Install directory
INSTALL_DIR := /opt/abmon

# Go build flags (for release)
GO_BUILD_FLAGS := -ldflags="-s -w"

.PHONY: build install \
        check_dns_propagation check_file_status check_http_redirect check_imap_message_age \
        check_becs_dhcp_scope check_ldap_auth check_gonemaster \
        check_radius_auth check_ntp_peers check_rrsig_expiry

build: check_dns_propagation check_gonemaster \
       check_file_status check_http_redirect check_imap_message_age check_becs_dhcp_scope check_ldap_auth \
       check_radius_auth check_ntp_peers check_rrsig_expiry

check_dns_propagation:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_dns_propagation ./cmd/check_dns_propagation

check_file_status:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_file_status ./cmd/check_file_status

check_http_redirect:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_http_redirect ./cmd/check_http_redirect

check_imap_message_age:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_imap_message_age ./cmd/check_imap_message_age

check_becs_dhcp_scope:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_becs_dhcp_scope ./cmd/check_becs_dhcp_scope

check_ldap_auth:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_ldap_auth ./cmd/check_ldap_auth

check_radius_auth:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_radius_auth ./cmd/check_radius_auth

check_ntp_peers:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_ntp_peers ./cmd/check_ntp_peers

check_rrsig_expiry:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_rrsig_expiry ./cmd/check_rrsig_expiry

check_gonemaster:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_gonemaster cmd/check_gonemaster/check_gonemaster.go

install: build
	install -m 755 $(BUILD_DIR)/check_dns_propagation    $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_gonemaster         $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_file_status        $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_http_redirect      $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_imap_message_age   $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_becs_dhcp_scope    $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_ldap_auth          $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_radius_auth        $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_ntp_peers          $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_rrsig_expiry       $(INSTALL_DIR)
