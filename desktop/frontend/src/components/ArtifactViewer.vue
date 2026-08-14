<script lang="ts" setup>
import { useAppStore } from '../stores/app'
import { timeText } from '../utils'

const store = useAppStore()
</script>

<template>
  <Teleport to="body">
    <div v-if="store.artifactViewerOpen" class="modal-overlay" @click.self="store.closeArtifactViewer">
      <div class="modal-panel artifact-modal">
        <div class="modal-header">
          <div>
            <b>产物详情</b>
            <small>{{ store.artifactViewerArtifact?.kind }}</small>
          </div>
          <button class="modal-close" @click="store.closeArtifactViewer">✕</button>
        </div>
        <div class="modal-body" v-if="store.artifactViewerArtifact">
          <div class="artifact-meta">
            <span>类型：{{ store.artifactViewerArtifact.kind }}</span>
            <span>大小：{{ (store.artifactViewerArtifact.size_bytes / 1024).toFixed(1) }} KB</span>
            <span>哈希：{{ store.artifactViewerArtifact.content_hash?.slice(0, 16) }}…</span>
            <span>创建：{{ timeText(store.artifactViewerArtifact.created_at) }}</span>
          </div>
          <pre v-if="store.artifactViewerArtifact.summary" class="artifact-summary">{{ store.artifactViewerArtifact.summary }}</pre>
          <pre class="artifact-content">{{ store.artifactViewerContent }}</pre>
        </div>
      </div>
    </div>
  </Teleport>
</template>
