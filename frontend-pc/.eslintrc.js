module.exports = {
  env: {
    browser: true,
    es2021: true,
    node: true,
  },
  extends: [
    'eslint:recommended',
    'plugin:react/recommended',
    'plugin:react-hooks/recommended',
  ],
  parserOptions: {
    ecmaFeatures: { jsx: true },
    ecmaVersion: 'latest',
    sourceType: 'module',
  },
  plugins: ['react', 'react-hooks'],
  rules: {
    'react/prop-types': 'off',
    'react-hooks/set-state-in-effect': 'warn',
    'no-unused-vars': 'warn',
    'no-undef': 'off',
  },
  globals: {
    localStorage: 'readonly',
    sessionStorage: 'readonly',
    fetch: 'readonly',
    FormData: 'readonly',
    Blob: 'readonly',
    URL: 'readonly',
    URLSearchParams: 'readonly',
    setTimeout: 'readonly',
    clearTimeout: 'readonly',
    setInterval: 'readonly',
    clearInterval: 'readonly',
  },
  settings: {
    react: { version: 'detect' },
  },
};
