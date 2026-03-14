export const instruments = [
  {
    id: 1,
    name: "雅马哈 U1 立式钢琴",
    category: "钢琴",
    image: "https://via.placeholder.com/300x200?text=Piano",
    monthlyRent: 800,
    deposit: 15000,
    minRentPeriod: 3,
    description: "日本进口，音色优美，适合初学者和专业人士",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除"
  },
  {
    id: 2,
    name: "卡马 F1 民谣吉他",
    category: "吉他",
    image: "https://via.placeholder.com/300x200?text=Guitar",
    monthlyRent: 150,
    deposit: 2000,
    minRentPeriod: 1,
    description: "单板面板，手感舒适，适合弹唱",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除"
  },
  {
    id: 3,
    name: "敦煌 694KK 古筝",
    category: "古筝",
    image: "https://via.placeholder.com/300x200?text=Guzheng",
    monthlyRent: 300,
    deposit: 5000,
    minRentPeriod: 1,
    description: "专业演奏级，音色清脆，外观精美",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除"
  },
  {
    id: 4,
    name: "铃木小提琴 SV-200",
    category: "提琴",
    image: "https://via.placeholder.com/300x200?text=Violin",
    monthlyRent: 200,
    deposit: 3500,
    minRentPeriod: 1,
    description: "实木手工制作，音质温暖，适合入门学习",
    depositNote: "押金在归还乐器无损后7个工作日内退还",
    wearStandard: "正常使用磨损不计入赔偿，严重损坏按维修费用扣除"
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
    image: "https://via.placeholder.com/150x150?text=MyPiano",
    startDate: "2026-01-15",
    endDate: "2026-07-15",
    rentMonths: 6,
    status: "租赁中",
    monthlyRent: 800
  }
];

export const nearbySites = [
  { id: 1, name: "朝阳区门店", distance: 1.2, address: "建国路88号" },
  { id: 2, name: "海淀区门店", distance: 3.5, address: "中关村大街1号" },
  { id: 3, name: "西城区门店", distance: 5.8, address: "金融街7号" }
];