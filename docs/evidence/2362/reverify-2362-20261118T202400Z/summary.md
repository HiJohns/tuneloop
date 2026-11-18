# #2362 Verification Result

- request_id: 2362-20261118T202400Z
- fix_commit: c1a4fb014bd5e1ab9400e177d96d6b4e5e36f6ee
- verification_head: c1a4fb014bd5e1ab9400e177d96d6b4e5e36f6ee
- host: ip-172-31-5-179
- node: v22.23.5
- go: go1.22.5 linux/amd64

## Commands & exit codes

```
r1-order-snapshot exit=0
r2-cap exit=0
r3-order-reject exit=0
r4-order-amount exit=0
r5-build-h5 exit=0
r6-build-weapp exit=0
```

## PASS / 关键输出

```
--- PASS: TestOrderSnapshotRatio_2362 (1.29s)
--- PASS: TestPointsCapParity_2362 (0.00s)
--- PASS: TestOrderRejectsBadPoints_2362 (1.20s)
--- PASS: TestOrderAmountConsistency_2362 (1.23s)
WXML_PROBE PASS
webpack 5.91.0 compiled with [1m[33m2 warnings[39m[22m in 279056 ms
```
