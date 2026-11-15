import { useEffect, useState } from "react";
import { Drawer } from "./components/Drawer";
import { AppsPage } from "./pages/AppsPage";
import { GoalsPage } from "./pages/GoalsPage";
import { HomePage } from "./pages/HomePage";
import { LearningPage } from "./pages/LearningPage";
import { LocalPage } from "./pages/LocalPage";
import { SettingsPage } from "./pages/SettingsPage";
import { SkillsPage } from "./pages/SkillsPage";
import { VersionPage } from "./pages/VersionPage";
import { RecentPage } from "./pages/RecentPage";

export type View =
  | "home"
  | "apps"
  | "goals"
  | "learning"
  | "local"
  | "skills"
  | "settings"
  | "version"
  | "recent";

export default function App() {
  const [view, setView] = useState<View>("home");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [tablet, setTablet] = useState(() => window.matchMedia("(min-width: 768px)").matches);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    const query = window.matchMedia("(min-width: 768px)");
    const sync = (event: MediaQueryListEvent | MediaQueryList) => setTablet(event.matches);
    sync(query);
    query.addEventListener("change", sync);
    return () => query.removeEventListener("change", sync);
  }, []);

  useEffect(() => {
    if (!notice) {
      return;
    }
    const timer = window.setTimeout(() => setNotice(""), 2600);
    return () => window.clearTimeout(timer);
  }, [notice]);

  const open = (next: View) => {
    setView(next);
    setDrawerOpen(false);
  };

  return (
    <div className="phone">
      <div className="screen">
        {view === "home" && (
          <HomePage onOpen={open} onOpenDrawer={() => setDrawerOpen(true)} onNotice={setNotice} />
        )}

        {view !== "home" && (
          <div className="page">
            {view === "apps" && <AppsPage onBack={() => setView("home")} onNotice={setNotice} />}
            {view === "goals" && <GoalsPage onBack={() => setView("home")} onNotice={setNotice} />}
            {view === "learning" && (
              <LearningPage onBack={() => setView("home")} onNotice={setNotice} />
            )}
            {view === "local" && <LocalPage onBack={() => setView("home")} onNotice={setNotice} />}
            {view === "skills" && <SkillsPage onBack={() => setView("home")} onNotice={setNotice} />}
            {view === "settings" && (
              <SettingsPage onBack={() => setView("home")} onNotice={setNotice} />
            )}
            {view === "version" && (
              <VersionPage onBack={() => setView("home")} onNotice={setNotice} />
            )}
            {view === "recent" && <RecentPage onBack={() => setView("home")} onNotice={setNotice} />}
          </div>
        )}

        <Drawer
          open={drawerOpen || tablet}
          current={view}
          onClose={() => setDrawerOpen(false)}
          onNavigate={open}
          onNewChat={() => {
            setDrawerOpen(false);
            setView("home");
            setNotice("已回到主对话，可以开始新的提问。");
          }}
          onNotice={setNotice}
        />

        {notice && <div className="toast">{notice}</div>}
      </div>
    </div>
  );
}
