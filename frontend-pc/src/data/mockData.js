export const assets = [
  {
    id: "TL-PI-2026-081",
    name: "雅马哈 U1 立式钢琴",
    category: "钢琴",
    level: "大师级",
    status: "在租",
    site: "TuneLoop 总店",
    siteId: "Site-001"
  },
  {
    id: "TL-GT-2026-042",
    name: "卡马 F1 民谣吉他",
    category: "吉他",
    level: "专业级",
    status: "待租",
    site: "Thornhill 分店",
    siteId: "Site-002"
  },
  {
    id: "TL-GZ-2026-015",
    name: "敦煌 694KK 古筝",
    category: "古筝",
    level: "入门级",
    status: "维修中",
    site: "TuneLoop 总店",
    siteId: "Site-001",
    workOrder: {
      id: "WO-001",
      jumps: 3,
      technician: "张师傅"
    }
  },
  {
    id: "TL-VN-2026-028",
    name: "铃木小提琴 SV-200",
    category: "提琴",
    level: "专业级",
    status: "待清理",
    site: "Thornhill 分店",
    siteId: "Site-002"
  }
];

export const financeConfig = {
  rentDepositRatio: {
    "入门级": 0.1,
    "专业级": 0.15,
    "大师级": 0.2
  },
  renewalDiscount: {
    "3个月": 1.0,
    "6个月": 0.95,
    "12个月": 0.9
  }
};
