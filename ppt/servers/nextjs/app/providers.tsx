'use client';

import { Provider } from 'react-redux';
import { store } from '../store/store';
import ChatGptAuthRedirectHandler from './ChatGptAuthRedirectHandler';
import { UiLanguageProvider } from '@/components/UiLanguage';

export function Providers({ children }: { children: React.ReactNode }) {
  return <Provider store={store}><UiLanguageProvider>
      <ChatGptAuthRedirectHandler />
      {children}
  </UiLanguageProvider></Provider>;
}
