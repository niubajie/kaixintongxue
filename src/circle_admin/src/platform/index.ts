// 平台能力抽象：管理端目前只跑在浏览器里，
// 但按 AGENTS.md 要求，存储能力统一从 bridge 取，禁止组件直接用 localStorage。
// 注意：Token 存 sessionStorage（关闭标签页即失效），不放 localStorage。

export interface SecureStorage {
  get(key: string): string | null;
  set(key: string, value: string): void;
  remove(key: string): void;
}

const memory = new Map<string, string>();

const webStorage: SecureStorage = {
  get(key) {
    try {
      return window.sessionStorage.getItem(key) ?? memory.get(key) ?? null;
    } catch {
      return memory.get(key) ?? null;
    }
  },
  set(key, value) {
    memory.set(key, value);
    try {
      window.sessionStorage.setItem(key, value);
    } catch {
      /* 隐私模式下忽略，退化为内存 */
    }
  },
  remove(key) {
    memory.delete(key);
    try {
      window.sessionStorage.removeItem(key);
    } catch {
      /* ignore */
    }
  }
};

export const bridge = {
  secureStorage: webStorage
};
