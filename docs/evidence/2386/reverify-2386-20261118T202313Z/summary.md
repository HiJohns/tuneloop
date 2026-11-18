# #2386 Verification Result

- request_id: 2386-20261118T202313Z
- fix_commit: 5929b0912d7de3ce68796017faec883b05a7bf3f（已合入 develop）
- verification_head: f807e028dbea511da4159adda1f008597fd4201f
- host: ip-172-31-5-179
- go: go version go1.22.5 linux/amd64

## Commands & exit codes

```
r1-aggregate exit=0
r2-scope exit=0
r3-platform exit=0
r4-build exit=0
r5-default-scope exit=0
r6-identity-roles exit=0
r7-merchant-role exit=0
r8-direct-isolation exit=0
```

## PASS lines

```
--- PASS: TestStaffMembersAggregate_2386 (1.39s)
--- PASS: TestStaffMembersScope_2386 (1.16s)
--- PASS: TestStaffMembersPlatform_2386 (1.08s)
--- PASS: TestStaffMembersDefaultScope_2386 (1.16s)
--- PASS: TestStaffMembersIdentityRoles_2386 (1.43s)
--- PASS: TestStaffMembersMerchantRole_2386 (1.16s)
--- PASS: TestStaffMembersDirectIsolation_2386 (1.31s)
```
