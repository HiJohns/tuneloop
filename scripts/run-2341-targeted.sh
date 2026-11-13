#!/usr/bin/env bash
# R6: 14 targeted tests (#2336 11 + 2 order-dependent + personnel cross-tenant negative).
# The 4 migration-chain tests are excluded: they belong to #2342.
set -euo pipefail
source scripts/lib-2341-env.sh
require_pg
echo "PROVENANCE $PROVENANCE"
REGEX='^(TestConfirmationSessionHandler_Confirm|TestDiscountCodeFlow|TestIAMProxyHandler_Issue340_IntegrationFlow|TestCancelOrderByCustomer_StatusGuard|TestGuestBatchCreateOrder|TestOrgManagement|TestPersonnel2067_MembershipAnchorAndDirect|TestPrepayMembershipWithCoupon_OREZ|TestPaymentCallback_RegistrationComplete|TestPaymentCallback_SessionFlow_RealCallback|TestRenewal_Confirm_OpenIDBackfilled|TestStaffOrderActions|TestUpdateUser_SiteIDNoChange|TestPersonnel2065_MerchantView)$'
( cd backend && go test ./handlers/ -run "$REGEX" -count=1 -timeout 10m 2>&1)
echo "R6 PASS (excluded: TestCentsMigration_20260820001, TestGiftPolicyReferralPresetMigration, TestPromoOverride_CheckConstraintMigration, TestPromoOverride_CrossMerchantForbidden_2260 -> #2342)"
