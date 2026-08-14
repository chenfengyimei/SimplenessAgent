/// <reference types="vite/client" />

interface Window {
  go: { main: { App: typeof import('../wailsjs/go/main/App') } }
}

declare module '*.vue' {
    import type {DefineComponent} from 'vue'
    const component: DefineComponent<{}, {}, any>
    export default component
}
