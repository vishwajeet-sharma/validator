import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from './context/ThemeContext';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { NewIdeaRefinement } from './pages/NewIdeaRefinement';
import { IdeaDetailDashboard } from './pages/IdeaDetailDashboard';

function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/new" element={<NewIdeaRefinement />} />
            <Route path="/refine/:id" element={<NewIdeaRefinement />} />
            <Route path="/idea/:id" element={<IdeaDetailDashboard />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </ThemeProvider>
  );
}

export default App;
