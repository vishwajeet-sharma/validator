import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from './context/ThemeContext';
import { IdeaProvider } from './context/IdeaContext';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { NewIdea } from './pages/NewIdea';
import { IdeaDetail } from './pages/IdeaDetail';

function App() {
  return (
    <ThemeProvider>
      <IdeaProvider>
        <BrowserRouter>
          <Layout>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/new" element={<NewIdea />} />
              <Route path="/idea/:id" element={<IdeaDetail />} />
            </Routes>
          </Layout>
        </BrowserRouter>
      </IdeaProvider>
    </ThemeProvider>
  );
}

export default App;
