export const instruments = [
  {
    id: 1,
    name: "雅马哈 U1 立式钢琴",
    category: "钢琴",
    image: "https://picsum.photos/seed/piano/400/400",
    minRentPeriod: 3,
    description: "日本进口，音色优美，适合初学者和专业人士",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除",
    sn: "TL-PI-2026-081",
    site: "TuneLoop 总店",
    lat: 43.8118,
    lng: -79.4231,
    distance: 1.5,
    levels: [
      { name: "入门级", monthlyRent: 199, deposit: 3000, maintenance: ["外观清洗", "基础调律"] },
      { name: "专业级", monthlyRent: 399, deposit: 8000, maintenance: ["深度维护", "精细调律"] },
      { name: "大师级", monthlyRent: 899, deposit: 20000, maintenance: ["大师级养护", "专家调律"] }
    ]
  },
  {
    id: 2,
    name: "卡马 F1 民谣吉他",
    category: "吉他",
    image: "https://picsum.photos/seed/guitar/400/400",
    minRentPeriod: 1,
    description: "单板面板，手感舒适，适合弹唱",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除",
    sn: "TL-GT-2026-042",
    site: "Thornhill 分店",
    lat: 43.8118,
    lng: -79.4231,
    distance: 1.5,
    levels: [
      { name: "入门级", monthlyRent: 99, deposit: 1500, maintenance: ["外观清洗", "基础调律"] },
      { name: "专业级", monthlyRent: 199, deposit: 4000, maintenance: ["深度维护", "精细调律"] },
      { name: "大师级", monthlyRent: 499, deposit: 10000, maintenance: ["大师级养护", "专家调律"] }
    ]
  },
  {
    id: 3,
    name: "敦煌 694KK 古筝",
    category: "古筝",
    image: "https://picsum.photos/seed/guzheng/400/400",
    minRentPeriod: 1,
    description: "专业演奏级，音色清脆，外观精美",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除",
    sn: "TL-GZ-2026-015",
    site: "TuneLoop 总店",
    lat: 43.8118,
    lng: -79.4231,
    distance: 1.5,
    levels: [
      { name: "入门级", monthlyRent: 150, deposit: 2500, maintenance: ["外观清洗", "基础调律"] },
      { name: "专业级", monthlyRent: 300, deposit: 5000, maintenance: ["深度维护", "精细调律"] },
      { name: "大师级", monthlyRent: 600, deposit: 12000, maintenance: ["大师级养护", "专家调律"] }
    ]
  },
  {
    id: 4,
    name: "铃木小提琴 SV-200",
    category: "提琴",
    image: "https://picsum.photos/seed/violin/400/400",
    minRentPeriod: 1,
    description: "实木手工制作，音质温暖，适合入门学习",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除",
    sn: "TL-VN-2026-028",
    site: "Thornhill 分店",
    lat: 43.8118,
    lng: -79.4231,
    distance: 1.5,
    levels: [
      { name: "入门级", monthlyRent: 120, deposit: 2000, maintenance: ["外观清洗", "基础调律"] },
      { name: "专业级", monthlyRent: 250, deposit: 4500, maintenance: ["深度维护", "精细调律"] },
      { name: "大师级", monthlyRent: 550, deposit: 10000, maintenance: ["大师级养护", "专家调律"] }
    ]
  }
];

export const categories = ["全部", "钢琴", "吉他", "古筝", "提琴"];

export const addresses = [
  { id: 1, name: "家庭地址", detail: "北京市朝阳区建国路88号", default: true },
  { id: 2, name: "公司地址", detail: "北京市海淀区中关村大街1号", default: false },
];

export const maintenancePackages = [
  {
    id: 1,
    name: "钢琴调律",
    type: "调律",
    price: 300,
    description: "专业调律师上门，包含音准调整、触感调整",
    duration: "1-2小时",
    icon: "🎹"
  },
  {
    id: 2,
    name: "深度清洁",
    type: "清洁",
    price: 200,
    description: "键盘、外壳、内部除尘，使用专业清洁剂",
    duration: "1小时",
    icon: "🧹"
  },
  {
    id: 3,
    name: "琴弦更换",
    type: "维修",
    price: 500,
    description: "更换单根/多根琴弦，含材料费",
    duration: "2小时",
    icon: "🎸"
  },
  {
    id: 4,
    name: "键盘维修",
    type: "维修",
    price: 400,
    description: "琴键不起/不弹回等机械故障修复",
    duration: "1-3小时",
    icon: "🔧"
  }
];

export const myAssets = [
  {
    id: 101,
    instrumentId: 1,
    name: "雅马哈 U1 立式钢琴",
    image: "https://images.unsplash.com/photo-1520529157262-d6c59239857d?q=80&w=400",
    startDate: "2026-01-15",
    endDate: "2026-07-15",
    rentMonths: 6,
    status: "租赁中",
    monthlyRent: 800
  }
];

export const myLeases = [
  {
    id: 1,
    instrumentName: "雅马哈 U1 立式钢琴",
    image: "https://images.unsplash.com/photo-1520529157262-d6c59239857d?q=80&w=400",
    startDate: "2026-01-15",
    endDate: "2026-07-15",
    status: "normal",
    monthlyRent: 800,
    daysLeft: 90,
    rentMonths: 3,
    totalMonths: 12
  },
  {
    id: 2,
    instrumentName: "铃木小提琴 SV-200",
    image: "https://images.unsplash.com/photo-1612228113110-3ac275ac34c0?q=80&w=400",
    startDate: "2025-12-01",
    endDate: "2026-03-21",
    status: "urgent",
    monthlyRent: 200,
    daysLeft: 7,
    rentMonths: 4,
    totalMonths: 12
  },
  {
    id: 3,
    instrumentName: "卡马 F1 民谣吉他",
    image: "https://picsum.photos/seed/guitar/400/400",
    startDate: "2025-09-01",
    endDate: "2025-11-01",
    status: "expired",
    monthlyRent: 199,
    daysLeft: 0,
    rentMonths: 6,
    totalMonths: 12
  },
  {
    id: 4,
    instrumentName: "敦煌 694KK 古筝",
    image: "https://picsum.photos/seed/guzheng/400/400",
    startDate: "2025-05-01",
    endDate: "2025-08-15",
    status: "熔断",
    monthlyRent: 300,
    daysLeft: 0,
    rentMonths: 8,
    totalMonths: 12
  }
];

export const depositRules = [
  { condition: "成色损耗 > 10%", penalty: "扣除¥500" },
  { condition: "琴键损坏", penalty: "扣除¥300" },
  { condition: "外观严重划痕", penalty: "扣除¥200" }
];

export const nearbySites = [
  { id: 1, name: "朝阳区门店", distance: 1.2, address: "建国路88号" },
  { id: 2, name: "海淀区门店", distance: 3.5, address: "中关村大街1号" },
  { id: 3, name: "西城区门店", distance: 5.8, address: "金融街7号" }
];

export const myServiceOrders = [
  {
    id: 1,
    assetName: "雅马哈 U1 立式钢琴",
    fault: "琴弦断裂",
    status: "待派单",
    site: "Site: 001 - 北京分店",
    createdAt: "2026-03-10"
  },
  {
    id: 2,
    assetName: "卡马 F1 民谣吉他",
    fault: "钢琴调律",
    status: "处理中",
    technician: "张师傅",
    technicianPhone: "138****8888",
    createdAt: "2026-03-08"
  }
];
