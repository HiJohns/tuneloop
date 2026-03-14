/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      screens: {
        'mobile': '480px'
      },
      colors: {
        brand: {
          primary: '#108ee9',
          bg: '#F5F5F9',
          text: '#1A1A1A',
          unit: '#A6A6A6',
          warning: '#faad14',
          danger: '#ff4d4f',
          success: '#52c41a'
        }
      }
    },
  },
  plugins: [],
}