# Build directory
BUILD_DIR := build

# Install directory
INSTALL_DIR := /usr/local/bin

# Go build flags (for release)
GO_BUILD_FLAGS := -ldflags="-s -w"

.PHONY: build install \
        check_dns_propagation check_file_status check_http_redirect check_imap_message_age \
        check_becs_dhcp_scope check_ldap_auth check_zonemaster create_icinga_zones_conf

build: check_dns_propagation check_zonemaster create_icinga_zones_conf \
       check_file_status check_http_redirect check_imap_message_age check_becs_dhcp_scope check_ldap_auth

check_dns_propagation:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_dns_propagation cmd/check_dns_propagation/check_dns_propagation.go cmd/check_dns_propagation/telnet_server.go

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

#check_ntp_peers:

#check_rrsig_expiry:

check_zonemaster:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/check_zonemaster cmd/check_zonemaster/check_zonemaster.go

create_icinga_zones_conf:
	@mkdir -p $(BUILD_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/create_icinga_zones_conf cmd/create_icinga_zones_conf/create_icinga_zones_conf.go

install: build
	install -m 755 $(BUILD_DIR)/check_dns_propagation    $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_zonemaster         $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/create_icinga_zones_conf $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_file_status        $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_http_redirect      $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_imap_message_age   $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_becs_dhcp_scope    $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/check_ldap_auth          $(INSTALL_DIR)
