package services

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// #1903: beaconiam signs the functional-roles claim as fn_roles; the previous
// "roles" tag parsed to an empty slice, so namespace admins were classified as
// merchant admins (tid==oid) and platform-staff management returned 403.
func TestJWTClaimsFnRolesMapping(t *testing.T) {
	var c JWTClaims
	require.NoError(t, json.Unmarshal([]byte(
		`{"sub":"u","tid":"t","oid":"t","role":"OWNER","fn_roles":["namespace_admin"]}`), &c))
	require.Equal(t, []string{"namespace_admin"}, c.Roles)
	require.Equal(t, "OWNER", c.Role)
}
