// #2437：weapp 页面级 DialogHost。Taro 页面是独立 React 根，只有挂到页面树内的
// DialogHost 才可见；App 层没有视图层（#2431 根因）。
import { useDidShow } from '@tarojs/taro'
import DialogHost, { notifyDialogHosts } from './DialogHost'

export default function PageShell({ children }) {
  useDidShow(() => notifyDialogHosts())
  return (
    <>
      {children}
      <DialogHost />
    </>
  )
}
