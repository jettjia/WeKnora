// 自动化模块入口: 路由懒加载本模块视图时通过副作用 import 注册模块 i18n
// (semantic 模块同款模式, 上游 locale 文件零改动)。
import { mergeLocaleMessage } from './messages'

mergeLocaleMessage()
