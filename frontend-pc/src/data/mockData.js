export const assets = [
  {
    id: "TL-PI-2026-081",
    name: "雅马哈 U1 立式钢琴",
    category: "钢琴",
    level: "大师级",
    status: "在租",
    site: "北京总店",
    siteId: "Site-001",
    value: 50000,
    leaseEnd: "2026-03-20",
    ownershipStatus: "租赁中",
    history: [
      { date: "2025-06-01", action: "出租", renter: "李四" },
      { date: "2025-08-15", action: "归还", renter: "李四" }
    ],
    repairCount: 0,
    workOrder: null
  },
  {
    id: "TL-GT-2026-042",
    name: "卡马 F1 民谣吉他",
    category: "吉他",
    level: "专业级",
    status: "待租",
    site: "上海分店",
    siteId: "Site-002",
    value: 3000,
    leaseEnd: null,
    ownershipStatus: "待租",
    history: [
      { date: "2025-09-01", action: "入库", renter: null }
    ],
    repairCount: 0,
    workOrder: null
  },
  {
    id: "TL-GZ-2026-015",
    name: "敦煌 694KK 古筝",
    category: "古筝",
    level: "入门级",
    status: "在租",
    site: "北京总店",
    siteId: "Site-001",
    value: 2000,
    leaseEnd: "2026-03-10",
    ownershipStatus: "已转售",
    history: [
      { date: "2025-06-01", action: "出租", renter: "王五" },
      { date: "2025-08-15", action: "归还", renter: "王五" },
      { date: "2025-09-01", action: "维修", note: "琴码调整", renter: null },
      { date: "2025-10-01", action: "维修", note: "琴弦更换", renter: null }
    ],
    repairCount: 2,
    workOrder: null
  },
  {
    id: "TL-VN-2026-028",
    name: "铃木小提琴 SV-200",
    category: "提琴",
    level: "专业级",
    status: "维修中",
    site: "上海分店",
    siteId: "Site-002",
    value: 8000,
    leaseEnd: "2026-03-15",
    ownershipStatus: "租赁中",
    history: [
      { date: "2025-07-01", action: "出租", renter: "赵六" },
      { date: "2025-10-01", action: "归还", renter: "赵六" },
      { date: "2025-10-15", action: "维修", note: "琴弓更换", renter: null }
    ],
    repairCount: 1,
    workOrder: null
  },
  {
    id: "TL-DR-2026-055",
    name: "罗兰 TD-17 电子鼓",
    category: "鼓",
    level: "专业级",
    status: "在租",
    site: "北京总店",
    siteId: "Site-001",
    value: 15000,
    leaseEnd: "2026-03-25",
    ownershipStatus: "租赁中",
    history: [
      { date: "2025-11-01", action: "出租", renter: "孙七" }
    ],
    repairCount: 0,
    workOrder: null
  },
  {
    id: "TL-KP-2026-012",
    name: "卡哇伊 K-300 立式钢琴",
    category: "钢琴",
    level: "大师级",
    status: "在租",
    site: "北京总店",
    siteId: "Site-001",
    value: 45000,
    leaseEnd: "2026-03-18",
    ownershipStatus: "租赁中",
    history: [
      { date: "2025-08-01", action: "出租", renter: "周八" }
    ],
    repairCount: 0,
    workOrder: null
  },
  {
    id: "TL-SY-2026-033",
    name: "雅马哈 YZ-125 萨克斯",
    category: "管乐",
    level: "专业级",
    status: "已熔断",
    site: "维修供应商",
    siteId: "Site-003",
    value: 12000,
    leaseEnd: null,
    ownershipStatus: "已转售",
    history: [
      { date: "2025-05-01", action: "出租", renter: "陈九" },
      { date: "2025-09-01", action: "归还", renter: "陈九" },
      { date: "2025-09-15", action: "维修", note: "按键损坏", renter: null },
      { date: "2025-10-01", action: "熔断", note: "无法修复，已熔断处理", renter: null }
    ],
    repairCount: 1,
    workOrder: null
  }
];

export const financeConfig = {
  levels: {
    "入门级": { rent: 299, deposit: 1000, renewalDiscount: 0.95 },
    "专业级": { rent: 599, deposit: 3000, renewalDiscount: 0.9 },
    "大师级": { rent: 1299, deposit: 8000, renewalDiscount: 0.85 }
  }
};
