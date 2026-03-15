import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Home from './pages/Home'
import Detail from './pages/Detail'
import Checkout from './pages/Checkout'
import Success from './pages/Success'
import Booking from './pages/Booking'
import Profile from './pages/Profile'
import MyService from './pages/MyService'

function App() {
  return (
    <BrowserRouter basename="/wx">
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/instrument/:id" element={<Detail />} />
        <Route path="/checkout/:id" element={<Checkout />} />
        <Route path="/success" element={<Success />} />
        <Route path="/booking" element={<Booking />} />
        <Route path="/booking/:assetId" element={<Booking />} />
        <Route path="/profile" element={<Profile />} />
        <Route path="/service" element={<MyService />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App