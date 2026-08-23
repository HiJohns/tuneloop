-- #1738 audit fix: settlement_calculations.result must preserve the EXACT
-- bytes computed by the handler — the acceptance contract is "what the
-- client sees is what the audit trail stores" (response data must be
-- byte-identical to the persisted result). JSONB normalizes stored values
-- (key reordering + whitespace), making byte-level identity impossible.
-- TEXT preserves the computed JSON verbatim; input_snapshot keeps JSONB
-- (semantic queries only, no byte-identity requirement).
ALTER TABLE settlement_calculations ALTER COLUMN result TYPE TEXT;
