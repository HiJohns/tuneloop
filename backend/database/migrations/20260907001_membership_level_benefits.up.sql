-- #1830: 会员权益可配置化 — 每档会员一条权益行（标题+描述），PC 管理端维护，移动端按 level_id 渲染
CREATE TABLE IF NOT EXISTS membership_level_benefits (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  level_id    int NOT NULL,
  sort_order  int NOT NULL DEFAULT 0,
  title       varchar(100) NOT NULL,
  description varchar(500) NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_membership_level_benefits_level ON membership_level_benefits (level_id);

-- 种子权益文案（草案，与当前 rebate_config/gift_policies 真实配置一致；上线前可由 PC 运营修改）
INSERT INTO membership_level_benefits (level_id, sort_order, title, description) VALUES
  (1, 1, '租金返现', '每笔实付租单结算完成后，按当前档位返现比例赠送积分（当前 0.5%）。'),
  (1, 2, '积分抵用', '下单支付时可按当期政策使用积分抵扣租金。'),
  (2, 1, '租金返现', '每笔实付租单结算完成后，按当前档位返现比例赠送积分（当前 1%）。'),
  (2, 2, '积分抵用', '下单支付时可按当期政策使用积分抵扣租金。'),
  (2, 3, '专属客服', '享受专属客服通道，问题优先响应。'),
  (3, 1, '租金返现', '每笔实付租单结算完成后，按当前档位返现比例赠送积分（当前 2%）。'),
  (3, 2, '积分抵用', '下单支付时可按当期政策使用积分抵扣租金。'),
  (3, 3, '专属客服', '享受专属客服通道，问题优先响应。'),
  (3, 4, '优先通道', '发货/售后优先处理绿色通道。')
ON CONFLICT DO NOTHING;
