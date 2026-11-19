# #2405 Verification Result

- request_id: 2405-20261119T053354Z
- fix_commit: 9a71e542771f09f2597c54051ef5d8a14574e690
- verification_head: 9a71e542771f09f2597c54051ef5d8a14574e690
- host: ip-172-31-5-179
- go: go1.22.5 linux/amd64

## Commands & exit codes

```
r1-merchant-staff exit=0
r2-technician exit=0
r3-idempotent exit=0
r4-site-regression exit=0
r5-build exit=0
r6-cross-merchant exit=0
r7-cross-tenant-tech exit=0
```

## PASS / BUILD

```
--- PASS: TestAcceptInviteMerchantStaff_2405 (1.11s)
--- PASS: TestAcceptInviteTechnician_2405 (1.23s)
--- PASS: TestAcceptInviteIdempotent_2405 (1.27s)
--- PASS: TestAcceptInviteSiteRegression_2405 (1.22s)
--- PASS: TestAcceptInviteCrossMerchant_2405 (1.23s)
--- PASS: TestAcceptInviteCrossTenantTechnician_2405 (1.18s)
BUILD OK
```
