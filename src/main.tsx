import { createRoot } from 'react-dom/client'
import App from './app/App'
import './styles/main.css'
import './styles/simulation.css'
import './styles/dashboard.css'

createRoot(document.getElementById('root')!).render(<App />)
