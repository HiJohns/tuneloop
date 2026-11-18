// #2415 S1：DialogHost 单独成文件，供 app.tsx / App-H5.jsx 挂载；
// 实现与订阅式 API 位于 AppDialog.jsx（避免入口直接依赖组件内部细节）。
export { DialogHost, DialogHost as default } from './AppDialog'
