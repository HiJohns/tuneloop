-- #1908: merge the per-severity warning notification configs into a single
-- `warning_notification` key (enabled = any enabled; emails = union;
-- cooldown = max). Old keys are kept so the change is reversible manually.
DO $$
DECLARE
  merged_enabled boolean := false;
  merged_emails jsonb := '[]'::jsonb;
  merged_cooldown int := 0;
  rec RECORD;
BEGIN
  FOR rec IN
    SELECT setting_value FROM system_settings
    WHERE tenant_id = '00000000-0000-0000-0000-000000000000'
      AND setting_key IN ('warning_level_low_actions','warning_level_medium_actions','warning_level_high_actions')
      AND setting_value <> ''
      AND setting_value ~ '^\s*\{'
  LOOP
    IF COALESCE((rec.setting_value::jsonb->>'enabled')::boolean, false) THEN
      merged_enabled := true;
    END IF;
    merged_emails := merged_emails || COALESCE(rec.setting_value::jsonb->'emails', '[]'::jsonb);
    merged_cooldown := GREATEST(merged_cooldown, COALESCE((rec.setting_value::jsonb->>'cooldown_minutes')::int, 0));
  END LOOP;

  SELECT COALESCE(jsonb_agg(DISTINCT e), '[]'::jsonb) INTO merged_emails
  FROM jsonb_array_elements_text(merged_emails) AS e;

  IF merged_emails <> '[]'::jsonb OR merged_enabled THEN
    INSERT INTO system_settings (id, tenant_id, setting_key, setting_value, updated_at)
    VALUES (gen_random_uuid(), '00000000-0000-0000-0000-000000000000', 'warning_notification',
            jsonb_build_object('enabled', merged_enabled, 'emails', merged_emails, 'cooldown_minutes', merged_cooldown)::text,
            NOW())
    ON CONFLICT (tenant_id, setting_key) DO NOTHING;
  END IF;
END $$;
