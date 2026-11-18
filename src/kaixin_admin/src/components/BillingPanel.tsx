import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type { ModelPrice, RechargeRecord } from "../types";

interface Props {
  notify: (message: string) => void;
}

function yuan(cents: number): string {
  return (cents / 100).toFixed(2);
}

export function BillingPanel({ notify }: Props) {
  const [recharges, setRecharges] = useState<RechargeRecord[]>([]);
  const [prices, setPrices] = useState<ModelPrice[]>([]);
  const [account, setAccount] = useState("");
  const [amount, setAmount] = useState("");
  const [channel, setChannel] = useState("wechat");
  const [note, setNote] = useState("");
  const [model, setModel] = useState("");
  const [inputPrice, setInputPrice] = useState("");
  const [outputPrice, setOutputPrice] = useState("");

  const load = useCallback(async () => {
    try {
      const [r, p] = await Promise.all([api.recharges(), api.prices()]);
      setRecharges(r.items);
      setPrices(p.items);
    } catch (err) {
      notify(err instanceof Error ? err.message : "读取计费数据失败");
    }
  }, [notify]);

  useEffect(() => {
    void load();
  }, [load]);

  const addRecharge = async () => {
    const cents = Math.round(Number(amount) * 100);
    if (!account.trim() || !Number.isFinite(cents) || cents <= 0) {
      notify("请填写账号与正确金额。");
      return;
    }
    try {
      await api.addRecharge({ account: account.trim(), amountCents: cents, channel, note });
      setAmount("");
      setNote("");
      notify("已记录充值。");
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "充值失败");
    }
  };

  const savePrice = async () => {
    if (!model.trim()) {
      notify("请填写模型名称。");
      return;
    }
    try {
      await api.savePrice({
        model: model.trim(),
        inputPerMillionCents: Math.round(Number(inputPrice || 0) * 100),
        outputPerMillionCents: Math.round(Number(outputPrice || 0) * 100)
      });
      setModel("");
      setInputPrice("");
      setOutputPrice("");
      notify("价格已保存。");
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "保存价格失败");
    }
  };

  return (
    <section>
      <div className="card">
        <h3>记一笔充值</h3>
        <div className="grid">
          <label>
            账号
            <input value={account} onChange={(e) => setAccount(e.target.value)} placeholder="admin" />
          </label>
          <label>
            金额（元）
            <input value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="100" />
          </label>
          <label>
            渠道
            <select value={channel} onChange={(e) => setChannel(e.target.value)}>
              <option value="wechat">微信</option>
              <option value="alipay">支付宝</option>
              <option value="manual">人工</option>
            </select>
          </label>
          <label>
            备注
            <input value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
        </div>
        <button onClick={() => void addRecharge()}>记录充值</button>
      </div>

      <h3>充值记录</h3>
      <table className="table">
        <thead>
          <tr>
            <th>时间</th>
            <th>账号</th>
            <th>金额</th>
            <th>渠道</th>
            <th>状态</th>
            <th>备注</th>
          </tr>
        </thead>
        <tbody>
          {recharges.map((item) => (
            <tr key={item.id}>
              <td>{item.createdAt}</td>
              <td>{item.account}</td>
              <td>¥{yuan(item.amountCents)}</td>
              <td>{item.channel}</td>
              <td>{item.status}</td>
              <td>{item.note || "—"}</td>
            </tr>
          ))}
          {recharges.length === 0 && (
            <tr>
              <td colSpan={6}>还没有充值记录。</td>
            </tr>
          )}
        </tbody>
      </table>

      <div className="card">
        <h3>Token 价格（每百万 Token，单位：元）</h3>
        <div className="grid">
          <label>
            模型
            <input value={model} onChange={(e) => setModel(e.target.value)} placeholder="Qwen3-Omni-Flash" />
          </label>
          <label>
            输入价格
            <input value={inputPrice} onChange={(e) => setInputPrice(e.target.value)} placeholder="0.01" />
          </label>
          <label>
            输出价格
            <input value={outputPrice} onChange={(e) => setOutputPrice(e.target.value)} placeholder="0.03" />
          </label>
        </div>
        <button onClick={() => void savePrice()}>保存价格</button>
      </div>

      <table className="table">
        <thead>
          <tr>
            <th>模型</th>
            <th>输入 / 百万</th>
            <th>输出 / 百万</th>
            <th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          {prices.map((item) => (
            <tr key={item.model}>
              <td>{item.model}</td>
              <td>¥{yuan(item.inputPerMillionCents)}</td>
              <td>¥{yuan(item.outputPerMillionCents)}</td>
              <td>{item.updatedAt}</td>
            </tr>
          ))}
          {prices.length === 0 && (
            <tr>
              <td colSpan={4}>还没有配置价格。</td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  );
}
