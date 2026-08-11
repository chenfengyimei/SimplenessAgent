/// <reference types="vite/client" />

interface Window { go: { main: { App: { ListTasks(): Promise<unknown[]> } } } }

declare module '*.vue' {
    import type {DefineComponent} from 'vue'
    const component: DefineComponent<{}, {}, any>
    export default component
}
