// Derived from https://raw.githubusercontent.com/prometheus/prometheus/1cffda5de775668f7438578c90a05e8bfa58eb91/web/ui/react-app/src/App.tsx.
// License: https://github.com/prometheus/prometheus/blob/1cffda5de775668f7438578c90a05e8bfa58eb91/LICENSE

import React, { FC } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import { BrowserRouter as Router, Route, Redirect } from 'react-router-dom';
import { PanelListPage } from './pages';
import { PathPrefixContext } from './contexts/PathPrefixContext';
import { ThemeContext, themeName, themeSetting } from './contexts/ThemeContext';
import { Theme, themeLocalStorageKey } from './Theme';
import { useLocalStorage } from './hooks/useLocalStorage';
import useMedia from './hooks/useMedia';

interface AppProps {
  consolesLink: string | null;
  agentMode: boolean;
}

const App: FC<AppProps> = ({ consolesLink, agentMode }) => {
  // This dynamically/generically determines the pathPrefix by stripping the first known
  // endpoint suffix from the window location path. It works out of the box for both direct
  // hosting and reverse proxy deployments with no additional configurations required.
  let basePath = window.location.pathname;
  const paths = ['/graph'];

  if (basePath.endsWith('/')) {
    basePath = basePath.slice(0, -1);
  }
  if (basePath.length > 1) {
    for (let i = 0; i < paths.length; i++) {
      if (basePath.endsWith(paths[i])) {
        basePath = basePath.slice(0, basePath.length - paths[i].length);
        break;
      }
    }
  }

  const [userTheme, setUserTheme] = useLocalStorage<themeSetting>(themeLocalStorageKey, 'auto');
  const browserHasThemes = useMedia('(prefers-color-scheme)');
  const browserWantsDarkTheme = useMedia('(prefers-color-scheme: dark)');

  let theme: themeName;
  if (userTheme !== 'auto') {
    theme = userTheme;
  } else {
    theme = browserHasThemes ? (browserWantsDarkTheme ? 'dark' : 'light') : 'light';
  }

  return (
    <ThemeContext.Provider
      value={{ theme: theme, userPreference: userTheme, setTheme: (t: themeSetting) => setUserTheme(t) }}
    >
      <Theme />
      <PathPrefixContext.Provider value={basePath}>
        <Router basename={basePath}>
          <Navigation consolesLink={consolesLink} agentMode={agentMode} />
          <Container fluid style={{ paddingTop: 70 }}>
            <Route path="/graph">
              <PanelListPage />
            </Route>
          </Container>
        </Router>
      </PathPrefixContext.Provider>
    </ThemeContext.Provider>
  );
};

export default App;
