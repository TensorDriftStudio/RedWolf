package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// AuthenticateLDAP validates user credentials against an OpenLDAP / FreeIPA directory server.
func AuthenticateLDAP(ctx context.Context, cfg domain.LDAPConfig, username, password string) (*domain.User, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("ldap authentication is disabled")
	}

	start := time.Now()
	conn, err := dialLDAP(cfg.Host, cfg.Port, cfg.UseTLS, cfg.StartTLS, cfg.InsecureSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDirectoryUnreachable, err)
	}
	defer conn.Close()

	// Step 1: Bind with service account if configured
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return nil, fmt.Errorf("service account bind failed: %w", err)
		}
	}

	// Step 2: Search for user DN and attributes
	userFilter := cfg.UserFilter
	if userFilter == "" {
		userFilter = "(&(objectClass=posixAccount)(uid=%s))"
	}
	searchFilter := fmt.Sprintf(userFilter, ldap.EscapeFilter(username))

	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 10, false,
		searchFilter,
		[]string{"dn", "cn", "mail", "displayName", "memberOf"},
		nil,
	)

	sr, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap user search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		slog.WarnContext(ctx, "ldap user not found", "username", username, "filter", searchFilter)
		return nil, domain.ErrInvalidCredentials
	}

	userEntry := sr.Entries[0]
	userDN := userEntry.DN

	// Step 3: Authenticate the user by binding with their user DN and password
	userConn, err := dialLDAP(cfg.Host, cfg.Port, cfg.UseTLS, cfg.StartTLS, cfg.InsecureSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDirectoryUnreachable, err)
	}
	defer userConn.Close()

	if err := userConn.Bind(userDN, password); err != nil {
		slog.WarnContext(ctx, "ldap user bind failed; invalid credentials", "dn", userDN)
		return nil, domain.ErrInvalidCredentials
	}

	// Step 4: Map user role from group membership
	role := resolveRole(userEntry.GetAttributeValues("memberOf"), cfg.AdminGroupDN, cfg.OperatorGroupDN)

	displayName := userEntry.GetAttributeValue("displayName")
	if displayName == "" {
		displayName = userEntry.GetAttributeValue("cn")
	}
	if displayName == "" {
		displayName = username
	}

	email := userEntry.GetAttributeValue("mail")

	slog.InfoContext(ctx, "ldap user authenticated successfully",
		"username", username,
		"dn", userDN,
		"role", role,
		"latency_ms", time.Since(start).Milliseconds(),
	)

	return &domain.User{
		ID:          "ldap-" + username,
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Role:        role,
		Source:      domain.AuthSourceLDAP,
		CreatedAt:   time.Now().UTC(),
		LastLoginAt: time.Now().UTC(),
	}, nil
}

// AuthenticateAD validates user credentials against Microsoft Active Directory.
func AuthenticateAD(ctx context.Context, cfg domain.ActiveDirectoryConfig, username, password string) (*domain.User, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("active directory authentication is disabled")
	}

	start := time.Now()
	host := cfg.DomainController
	if host == "" {
		host = cfg.Domain
	}
	port := cfg.Port
	if port <= 0 {
		if cfg.UseLDAPS {
			port = 636
		} else {
			port = 389
		}
	}

	conn, err := dialLDAP(host, port, cfg.UseLDAPS, false, cfg.InsecureSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDirectoryUnreachable, err)
	}
	defer conn.Close()

	// Step 1: Bind with service account
	bindDN := cfg.BindDN
	if bindDN != "" {
		if err := conn.Bind(bindDN, cfg.BindPassword); err != nil {
			return nil, fmt.Errorf("ad service account bind failed: %w", err)
		}
	}

	// Step 2: Search for user
	userFilter := cfg.UserSearchFilter
	if userFilter == "" {
		userFilter = "(&(objectClass=user)(|(sAMAccountName=%s)(userPrincipalName=%s)))"
	}
	searchFilter := fmt.Sprintf(userFilter, ldap.EscapeFilter(username), ldap.EscapeFilter(username))

	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 10, false,
		searchFilter,
		[]string{"dn", "cn", "mail", "displayName", "sAMAccountName", "memberOf"},
		nil,
	)

	sr, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ad user search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		slog.WarnContext(ctx, "ad user not found", "username", username, "filter", searchFilter)
		return nil, domain.ErrInvalidCredentials
	}

	userEntry := sr.Entries[0]
	userDN := userEntry.DN

	// Step 3: Validate user password by attempting to bind as user
	userConn, err := dialLDAP(host, port, cfg.UseLDAPS, false, cfg.InsecureSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDirectoryUnreachable, err)
	}
	defer userConn.Close()

	if err := userConn.Bind(userDN, password); err != nil {
		// Also try user@domain UPN format
		upn := username
		if !strings.Contains(upn, "@") && cfg.Domain != "" {
			upn = fmt.Sprintf("%s@%s", username, cfg.Domain)
		}
		if err2 := userConn.Bind(upn, password); err2 != nil {
			slog.WarnContext(ctx, "ad user bind failed; invalid credentials", "dn", userDN, "upn", upn)
			return nil, domain.ErrInvalidCredentials
		}
	}

	// Step 4: Map user role from group membership
	role := resolveRole(userEntry.GetAttributeValues("memberOf"), cfg.AdminGroup, cfg.OperatorGroup)

	displayName := userEntry.GetAttributeValue("displayName")
	if displayName == "" {
		displayName = userEntry.GetAttributeValue("cn")
	}
	if displayName == "" {
		displayName = username
	}

	email := userEntry.GetAttributeValue("mail")

	slog.InfoContext(ctx, "ad user authenticated successfully",
		"username", username,
		"dn", userDN,
		"role", role,
		"latency_ms", time.Since(start).Milliseconds(),
	)

	return &domain.User{
		ID:          "ad-" + username,
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Role:        role,
		Source:      domain.AuthSourceAD,
		CreatedAt:   time.Now().UTC(),
		LastLoginAt: time.Now().UTC(),
	}, nil
}

// TestLDAPConnection runs connectivity and search diagnostics against an LDAP server.
func TestLDAPConnection(ctx context.Context, cfg domain.LDAPConfig) *domain.DirectoryTestResult {
	start := time.Now()
	res := &domain.DirectoryTestResult{
		TestedAt: start.UTC(),
	}

	conn, err := dialLDAP(cfg.Host, cfg.Port, cfg.UseTLS, cfg.StartTLS, cfg.InsecureSkipVerify)
	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("Failed to connect to LDAP host %s:%d: %v", cfg.Host, cfg.Port, err)
		res.LatencyMs = time.Since(start).Milliseconds()
		return res
	}
	defer conn.Close()

	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			res.Success = false
			res.Message = fmt.Sprintf("LDAP Bind failed for DN '%s': %v", cfg.BindDN, err)
			res.LatencyMs = time.Since(start).Milliseconds()
			return res
		}
	}

	// Run search query in BaseDN
	searchFilter := "(objectClass=*)"
	if cfg.UserFilter != "" {
		searchFilter = fmt.Sprintf(cfg.UserFilter, "*")
	}

	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 10, 5, false,
		searchFilter,
		[]string{"dn"},
		nil,
	)

	sr, err := conn.Search(searchReq)
	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("LDAP search failed in BaseDN '%s': %v", cfg.BaseDN, err)
		res.LatencyMs = time.Since(start).Milliseconds()
		return res
	}

	res.Success = true
	res.EntriesFound = len(sr.Entries)
	res.LatencyMs = time.Since(start).Milliseconds()
	res.Message = fmt.Sprintf("Successfully connected and authenticated. Found %d directory entries.", len(sr.Entries))
	return res
}

// TestADConnection runs connectivity and search diagnostics against Active Directory.
func TestADConnection(ctx context.Context, cfg domain.ActiveDirectoryConfig) *domain.DirectoryTestResult {
	start := time.Now()
	res := &domain.DirectoryTestResult{
		TestedAt: start.UTC(),
	}

	host := cfg.DomainController
	if host == "" {
		host = cfg.Domain
	}
	port := cfg.Port
	if port <= 0 {
		if cfg.UseLDAPS {
			port = 636
		} else {
			port = 389
		}
	}

	conn, err := dialLDAP(host, port, cfg.UseLDAPS, false, cfg.InsecureSkipVerify)
	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("Failed to connect to Active Directory controller %s:%d: %v", host, port, err)
		res.LatencyMs = time.Since(start).Milliseconds()
		return res
	}
	defer conn.Close()

	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			res.Success = false
			res.Message = fmt.Sprintf("AD Bind failed for '%s': %v", cfg.BindDN, err)
			res.LatencyMs = time.Since(start).Milliseconds()
			return res
		}
	}

	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 10, 5, false,
		"(objectClass=user)",
		[]string{"dn"},
		nil,
	)

	sr, err := conn.Search(searchReq)
	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("Active Directory user search failed in '%s': %v", cfg.BaseDN, err)
		res.LatencyMs = time.Since(start).Milliseconds()
		return res
	}

	res.Success = true
	res.EntriesFound = len(sr.Entries)
	res.LatencyMs = time.Since(start).Milliseconds()
	res.Message = fmt.Sprintf("Successfully connected to Domain '%s'. Found %d directory objects.", cfg.Domain, len(sr.Entries))
	return res
}

func dialLDAP(host string, port int, useTLS, startTLS, insecureSkipVerify bool) (*ldap.Conn, error) {
	if port <= 0 {
		if useTLS {
			port = 636
		} else {
			port = 389
		}
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
		ServerName:         host,
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	var conn *ldap.Conn
	var err error

	if useTLS {
		conn, err = ldap.DialTLS("tcp", addr, tlsConfig)
	} else {
		conn, err = ldap.Dial("tcp", addr)
		if err == nil && startTLS {
			if tlsErr := conn.StartTLS(tlsConfig); tlsErr != nil {
				conn.Close()
				return nil, fmt.Errorf("starttls negotiation failed: %w", tlsErr)
			}
		}
	}

	return conn, err
}

func resolveRole(memberships []string, adminGroup, operatorGroup string) domain.UserRole {
	for _, m := range memberships {
		mLower := strings.ToLower(m)
		if adminGroup != "" && strings.Contains(mLower, strings.ToLower(adminGroup)) {
			return domain.RoleAdmin
		}
	}
	for _, m := range memberships {
		mLower := strings.ToLower(m)
		if operatorGroup != "" && strings.Contains(mLower, strings.ToLower(operatorGroup)) {
			return domain.RoleOperator
		}
	}
	return domain.RoleOperator
}
