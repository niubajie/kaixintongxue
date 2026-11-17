// 跨页面的运行时状态：目前只记「是否有部署任务在跑」。
// 关闭保护、导航提示等少数几处需要它；不值得为此引入全局 store。

type Listener = (deploying: boolean) => void;

let deploying = false;
const listeners = new Set<Listener>();

export function setDeploying(value: boolean): void {
  if (deploying === value) {
    return;
  }
  deploying = value;
  for (const listener of listeners) {
    listener(value);
  }
}

export function isDeploying(): boolean {
  return deploying;
}

export function subscribeDeploying(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
