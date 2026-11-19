// 业务场景技能依赖层级（L1 场景 → L2 细分技能 → L3 依赖）
// 数据来源：业务场景技能依赖层级表.xlsx
// 本文件由脚本生成，改数据请改 Excel 后重新生成，不要手改。

export type DependencyType = "本地软件" | "大模型" | "网站平台";
export type DependencyStatus = "已就绪" | "待接入" | "需授权";

export interface Dependency {
  type: DependencyType;
  name: string;
  purpose: string;
  status: DependencyStatus;
}

export interface SubSkill {
  name: string;
  desc: string;
  deps: Dependency[];
}

export interface BusinessScenario {
  name: string;
  skills: SubSkill[];
}

export interface ScenarioCategory {
  name: string;
  scenarios: BusinessScenario[];
}

export const SCENARIO_TREE: ScenarioCategory[] = [
  {
    "name": "工作类",
    "scenarios": [
      {
        "name": "电商运营自动化",
        "skills": [
          {
            "name": "商品信息采集与比价",
            "desc": "采集规格、价格与供应商信息。",
            "deps": [
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "竞品销量分析",
            "desc": "汇总竞品销量与评价趋势。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "素材采集",
            "desc": "收集商品图、视频与卖点素材。",
            "deps": [
              {
                "type": "本地软件",
                "name": "剪辑与图片工具",
                "purpose": "素材剪辑、封面配图、画面处理",
                "status": "待接入"
              },
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              }
            ]
          },
          {
            "name": "图文/短视频上架",
            "desc": "生成图文与短视频并上架。",
            "deps": [
              {
                "type": "本地软件",
                "name": "剪辑与图片工具",
                "purpose": "素材剪辑、封面配图、画面处理",
                "status": "待接入"
              },
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              }
            ]
          },
          {
            "name": "订单与评价跟踪",
            "desc": "跟踪订单状态、汇总客户评价。",
            "deps": [
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "数据日报",
            "desc": "汇总运营数据生成日报。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "多媒体内容自动化",
        "skills": [
          {
            "name": "图片/视频素材采集",
            "desc": "归集原始素材。",
            "deps": [
              {
                "type": "本地软件",
                "name": "剪辑与图片工具",
                "purpose": "素材剪辑、封面配图、画面处理",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "剪辑成片",
            "desc": "按脚本剪辑成片。",
            "deps": [
              {
                "type": "本地软件",
                "name": "剪辑与图片工具",
                "purpose": "素材剪辑、封面配图、画面处理",
                "status": "待接入"
              },
              {
                "type": "大模型",
                "name": "本地视觉模型",
                "purpose": "图片/视频画面识别理解",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "封面配图",
            "desc": "生成封面与配图。",
            "deps": [
              {
                "type": "本地软件",
                "name": "剪辑与图片工具",
                "purpose": "素材剪辑、封面配图、画面处理",
                "status": "待接入"
              },
              {
                "type": "大模型",
                "name": "本地视觉模型",
                "purpose": "图片/视频画面识别理解",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "文案配音",
            "desc": "生成配音文案并转语音。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "语音识别引擎",
                "purpose": "录音转写、语音交互",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "素材归档",
            "desc": "分类归档素材与成片。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          }
        ]
      },
      {
        "name": "维修 AI 辅助",
        "skills": [
          {
            "name": "故障现象识别",
            "desc": "根据现象判断故障方向。",
            "deps": [
              {
                "type": "大模型",
                "name": "本地视觉模型",
                "purpose": "图片/视频画面识别理解",
                "status": "待接入"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "元件/画面识别",
            "desc": "摄像头识别主板与元件。",
            "deps": [
              {
                "type": "大模型",
                "name": "本地视觉模型",
                "purpose": "图片/视频画面识别理解",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "语音识别引擎",
                "purpose": "录音转写、语音交互",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "维修方案建议",
            "desc": "给出步骤与备件建议。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "维修记录沉淀",
            "desc": "记录案例并入知识库。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "方案撰写自动化",
        "skills": [
          {
            "name": "需求整理",
            "desc": "梳理目标、约束与素材。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "资料汇编",
            "desc": "汇总参考资料与数据。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "方案大纲生成",
            "desc": "生成结构与章节大纲。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "成稿润色与版本对比",
            "desc": "润色成稿并对比版本。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "工作笔记",
        "skills": [
          {
            "name": "会议录音转写",
            "desc": "语音转文字。",
            "deps": [
              {
                "type": "本地软件",
                "name": "语音识别引擎",
                "purpose": "录音转写、语音交互",
                "status": "待接入"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "要点提炼",
            "desc": "提炼结论与行动项。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "笔记分类归档",
            "desc": "按主题归档。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "待办事项提取",
            "desc": "提取待办并提醒。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "客户关系管理（CRM）",
        "skills": [
          {
            "name": "客户信息建档",
            "desc": "建立客户档案。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "跟进记录",
            "desc": "记录沟通与进展。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "沟通话术",
            "desc": "生成跟进话术。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "到期提醒",
            "desc": "合同/回访到期提醒。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "客户分层",
            "desc": "按价值分层管理。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "财务记账自动化",
        "skills": [
          {
            "name": "票据识别录入",
            "desc": "识别票据并录入账目。",
            "deps": [
              {
                "type": "大模型",
                "name": "本地视觉模型",
                "purpose": "图片/视频画面识别理解",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "账目核对",
            "desc": "核对收支与流水。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "收支汇总",
            "desc": "汇总分类收支。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "财务报表生成",
            "desc": "生成报表与结论。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "采购流程自动化",
        "skills": [
          {
            "name": "需求汇总",
            "desc": "归集采购需求。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "供应商比价",
            "desc": "对比价格与条件。",
            "deps": [
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "采购单生成",
            "desc": "生成采购单据。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "到货与库存跟踪",
            "desc": "跟踪到货与库存。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "计划编排自动化",
        "skills": [
          {
            "name": "目标拆解",
            "desc": "拆解为可执行任务。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "任务排期",
            "desc": "安排时间与优先级。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "日程编排",
            "desc": "编排日程与提醒。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "进度跟踪与调整",
            "desc": "跟踪进度并调整。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "考勤统计自动化",
        "skills": [
          {
            "name": "打卡记录汇总",
            "desc": "汇总出勤记录。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "异常考勤识别",
            "desc": "识别缺卡/迟到异常。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "排班核对",
            "desc": "核对排班与出勤。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "考勤报表输出",
            "desc": "输出考勤报表。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "人事事务自动化",
        "skills": [
          {
            "name": "招聘信息整理",
            "desc": "整理岗位与渠道信息。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              }
            ]
          },
          {
            "name": "简历初筛",
            "desc": "按条件初筛简历。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "入离职手续",
            "desc": "整理手续清单与记录。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "绩效与档案记录",
            "desc": "记录绩效与档案。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "职业发展规划",
        "skills": [
          {
            "name": "能力盘点",
            "desc": "盘点现有技能与短板。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "方向定位",
            "desc": "结合兴趣与市场定位方向。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "学习路径",
            "desc": "规划学习路径与阶段目标。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "目标与复盘",
            "desc": "设定目标并阶段复盘。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      }
    ]
  },
  {
    "name": "生活类",
    "scenarios": [
      {
        "name": "健康与作息管理",
        "skills": [
          {
            "name": "体征/饮食记录",
            "desc": "记录饮食与身体数据。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "作息提醒",
            "desc": "作息与饮水提醒。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "运动计划",
            "desc": "制定运动计划。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "健康资料归档",
            "desc": "归档健康资料。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          }
        ]
      },
      {
        "name": "旅行行程规划",
        "skills": [
          {
            "name": "目的地攻略",
            "desc": "收集目的地攻略。",
            "deps": [
              {
                "type": "网站平台",
                "name": "地图/攻略站点",
                "purpose": "目的地信息与行程参考",
                "status": "需授权"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "行程编排",
            "desc": "编排每日行程。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "预算核算",
            "desc": "核算交通住宿预算。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "出行清单",
            "desc": "生成出行清单。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "游记整理",
            "desc": "整理游记与照片。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "剪辑与图片工具",
                "purpose": "素材剪辑、封面配图、画面处理",
                "status": "待接入"
              }
            ]
          }
        ]
      },
      {
        "name": "人情往来管理",
        "skills": [
          {
            "name": "往来记录",
            "desc": "记录往来事项。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "礼金/礼物台账",
            "desc": "登记礼金礼物台账。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "生日节日提醒",
            "desc": "重要日期提醒。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "寒暄话术参考",
            "desc": "生成问候话术。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "购物比价与采购管理",
        "skills": [
          {
            "name": "商品比价",
            "desc": "对比价格与优惠。",
            "deps": [
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "规格对比",
            "desc": "对比规格与评价。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "下单跟踪",
            "desc": "跟踪订单物流。",
            "deps": [
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              }
            ]
          },
          {
            "name": "家庭采购清单",
            "desc": "维护采购清单。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "优惠券管理",
        "skills": [
          {
            "name": "优惠券归集",
            "desc": "归集可用优惠券。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "网站平台",
                "name": "电商平台",
                "purpose": "商品信息采集与内容发布",
                "status": "需授权"
              }
            ]
          },
          {
            "name": "到期提醒",
            "desc": "券到期提醒。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "凑单建议",
            "desc": "给出凑单建议。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "优惠抵扣测算",
            "desc": "测算抵扣后价格。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              }
            ]
          }
        ]
      }
    ]
  },
  {
    "name": "学习类",
    "scenarios": [
      {
        "name": "课程跟学",
        "skills": [
          {
            "name": "课程进度跟踪",
            "desc": "跟踪学习进度。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "笔记整理",
            "desc": "整理课程笔记。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "知识点提炼",
            "desc": "提炼重点知识点。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "复习计划生成",
            "desc": "生成复习计划。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "试题刷题与错题",
        "skills": [
          {
            "name": "题库练习",
            "desc": "按章节刷题。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "错题归集",
            "desc": "归集错题与原因。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "薄弱点分析",
            "desc": "分析薄弱知识点。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "针对性重复练习",
            "desc": "生成针对性练习。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "个人成长复盘",
        "skills": [
          {
            "name": "目标打卡",
            "desc": "每日目标打卡。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "阶段复盘",
            "desc": "阶段总结复盘。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "经验沉淀",
            "desc": "沉淀经验与方法。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "习惯养成跟踪",
            "desc": "跟踪习惯养成。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      },
      {
        "name": "职业技能学习",
        "skills": [
          {
            "name": "技能路径规划",
            "desc": "规划技能学习路径。",
            "deps": [
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              },
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              }
            ]
          },
          {
            "name": "教程跟练",
            "desc": "跟练教程与示例。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地向量知识库",
                "purpose": "文档检索与问答沉淀",
                "status": "待接入"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "项目实操",
            "desc": "完成实操项目。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地文档工具",
                "purpose": "读写文档、表格、周报草稿",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          },
          {
            "name": "成果检验",
            "desc": "检验学习成果。",
            "deps": [
              {
                "type": "本地软件",
                "name": "本地表格处理",
                "purpose": "清洗数据、汇总统计、生成报表",
                "status": "已就绪"
              },
              {
                "type": "大模型",
                "name": "云端文本大模型",
                "purpose": "文案、方案、总结与推理",
                "status": "已就绪"
              }
            ]
          }
        ]
      }
    ]
  }
];

export const DEPENDENCY_TYPES: DependencyType[] = ["本地软件", "大模型", "网站平台"];

export const DEPENDENCY_STATUSES: DependencyStatus[] = ["已就绪", "待接入", "需授权"];

/** 场景的唯一键：大类 + 场景名 */
export function scenarioKey(category: string, scenario: string): string {
  return `${category}::${scenario}`;
}

/** 技能的唯一键：大类 + 场景 + 技能名 */
export function skillKey(category: string, scenario: string, skill: string): string {
  return `${category}::${scenario}::${skill}`;
}

/** 汇总一个场景的依赖与就绪情况，用于卡片上的一行说明 */
export function scenarioSummary(scenario: BusinessScenario): { skills: number; deps: number; pending: number } {
  const deps = new Map<string, Dependency>();
  for (const skill of scenario.skills) {
    for (const dep of skill.deps) {
      deps.set(`${dep.type}::${dep.name}`, dep);
    }
  }
  let pending = 0;
  for (const dep of deps.values()) {
    if (dep.status !== "已就绪") {
      pending += 1;
    }
  }
  return { skills: scenario.skills.length, deps: deps.size, pending };
}
