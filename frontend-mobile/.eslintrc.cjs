module.exports = {
  root: true,
  env: {
    browser: true,
    es2021: true,
    node: true,
  },
  extends: [
    'eslint:recommended',
    'plugin:react-hooks/recommended',
  ],
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
    ecmaFeatures: {
      jsx: true,
    },
  },
  globals: {
    wx: 'readonly',
    BarcodeDetector: 'readonly',
  },
  rules: {
    'no-undef': 'error',
    'no-use-before-define': ['error', { variables: true, functions: false }],
    'no-unused-vars': ['warn', { varsIgnorePattern: '^[A-Z_]' }],
    'react-hooks/set-state-in-effect': 'off',
    'react/react-in-jsx-scope': 'off',
    'react/prop-types': 'off',
    'no-empty': 'off',
    'react-hooks/exhaustive-deps': 'warn',
    'react-hooks/preserve-manual-memoization': 'warn',
  },
}
