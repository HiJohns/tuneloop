# build=1.0.a1fd22f87 commit=a1fd22f87e909a585ba904552685094993eedb9d dirty=false
# R3 — 中文合计标签字距口径

- 结论：中文标签不保留英文大写排版字距（uppercase/tracking-widest/letterSpacing）。
- PASS: H5 标签无 uppercase/tracking-widest
- PASS: weapp 标签无 letterSpacing
- PASS: 无 TOTAL PAYABLE

## 证据
1084:            <Text className="text-xs text-zinc-400 font-bold block">{t('应付合计')}</Text>
1080:            <Text style={{ fontSize: 12, color: '#a1a1aa', fontWeight: '700' }}>{t('应付合计')}</Text>
TYPOGRAPHY_OK
