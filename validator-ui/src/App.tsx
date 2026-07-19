import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from './context/ThemeContext';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { NewIdeaForm } from './pages/NewIdeaForm';
import { IdeaDetailDashboard } from './pages/IdeaDetailDashboard';

function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/new" element={<NewIdeaForm />} />
            <Route path="/idea/:id" element={<IdeaDetailDashboard />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </ThemeProvider>
  );
}

export default App;
