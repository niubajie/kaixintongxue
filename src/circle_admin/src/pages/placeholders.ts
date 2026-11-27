export interface PlaceholderDef {
  path: string;
  title: string;
  desc: string;
  planned: string[];
}

// 订单与投诉模块依赖 S4/S5 的表结构（orders / complaints 尚未建表），
// 前端先占位并明确列出规划中的接口，不展示任何模拟数据。
export const PLACEHOLDERS: PlaceholderDef[] = [
  {
    path: "/orders",
    title: "订单退款",
    desc: "订单查询、退款发起与审批、对账差异处理（金额一律整数分）。",
    planned: [
      "GET /api/v1/admin/orders",
      "GET /api/v1/admin/orders/{uuid}",
      "POST /api/v1/admin/orders/{uuid}/refund",
      "GET /api/v1/admin/reconcile"
    ]
  },
  {
    path: "/complaints",
    title: "投诉处理",
    desc: "投诉工单列表、分配、回复、结案与超时提醒。",
    planned: [
      "GET /api/v1/admin/complaints",
      "POST /api/v1/admin/complaints/{uuid}/assign",
      "POST /api/v1/admin/complaints/{uuid}/reply",
      "POST /api/v1/admin/complaints/{uuid}/close"
    ]
  }
];
