
import { useT } from './i18n/locale';
import {
  useAgentBackendSync,
  useAppRoute,
  useProjects,
} from './app/appShell';
import { AppSplash, DashboardRoute, EditorRoute } from './app/AppViews';
import { useInferenceWarmup } from './hooks/useInferenceWarmup';
import { useUiScaleShortcuts } from './hooks/useUiScaleShortcuts';
import { LoginGate } from './gateway/LoginGate';
import { gatewayLoginRequired } from './gateway/gateway-mode';

function EditorApp() {
  const t = useT();
  const route = useAppRoute();
  useAgentBackendSync();
  useInferenceWarmup(route.name === 'editor');
  useUiScaleShortcuts();
  const { projects, refresh } = useProjects();

  if (!projects) return <AppSplash text={t('加载中…')} />;
  return route.name === 'editor'
    ? <EditorRoute route={route} projects={projects} refresh={refresh} />
    : <DashboardRoute projects={projects} refresh={refresh} />;
}

export default function App() {
  return gatewayLoginRequired()
    ? <LoginGate><EditorApp /></LoginGate>
    : <EditorApp />;
}
