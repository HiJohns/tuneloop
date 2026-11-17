# #2403 Verification Result

- request_id: 2403-20261117T143422Z
- fix_commit: 3cf057478eecaa7525d8c886b0f423daf2c7521b
- head: 2af70ee529112f1e75515ce6d5098ddd5e3a1363
- host: ip-172-31-5-179
- go: go version go1.22.5 linux/amd64

## Commands & exit codes

```
r1-invite exit=0
r2-isolation exit=0
r3-dedup exit=0
build exit=0
```

## PASS lines

```
--- PASS: TestInviteToMerchantMerchantScopeFixed_2403 (1.19s)
--- PASS: TestInviteToMerchantCrossMerchant404_2403 (1.16s)
--- PASS: TestInviteToMerchantDedupAndKind_2403 (1.17s)
```
