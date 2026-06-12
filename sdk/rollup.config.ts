import typescript from '@rollup/plugin-typescript';
import terser from '@rollup/plugin-terser';

export default {
  input: 'src/index.ts',
  output: [
    {
      file: 'dist/shieldcaptcha.cjs.js',
      format: 'cjs',
      sourcemap: true,
      exports: 'named',
    },
    {
      file: 'dist/shieldcaptcha.esm.js',
      format: 'es',
      sourcemap: true,
    },
    {
      file: 'dist/shieldcaptcha.umd.js',
      format: 'umd',
      name: 'ShieldCaptcha',
      sourcemap: true,
      plugins: [terser()],
    },
  ],
  plugins: [
    typescript({ tsconfig: './tsconfig.json' }),
  ],
};
