<script setup lang="ts">
import SvgIcon from "@/components/icons/SvgIcon.vue";
import {
  APP_URL,
  APP_VERSION_BADGE,
  COLLAB_BADGE_TEXT,
  COLLAB_URL,
  GITHUB_URL,
} from "@/version";

const badges = [
  {
    key: "docs",
    href: APP_URL,
    icon: "globe",
    label: "当前版本",
    value: APP_VERSION_BADGE,
  },
  {
    key: "github",
    href: GITHUB_URL,
    icon: "github",
    label: "项目地址",
    value: "Github 仓库",
  },
  {
    key: "collab",
    href: COLLAB_URL,
    icon: "bilibili",
    label: "联合测评",
    value: COLLAB_BADGE_TEXT,
  },
] as const;
</script>

<template>
  <footer class="footer">
    <div class="container footer__inner">
      <template v-for="(item, index) in badges" :key="item.key">
        <span v-if="index > 0" class="footer__sep" aria-hidden="true">|</span>
        <a
          class="footer-item"
          :href="item.href"
          target="_blank"
          rel="noopener noreferrer"
        >
          <SvgIcon :name="item.icon" :size="14" class-name="footer-item__icon" />
          <span class="footer-item__label">{{ item.label }}</span>
          <span class="footer-item__value">{{ item.value }}</span>
        </a>
      </template>
    </div>
  </footer>
</template>

<style scoped>
.footer {
  margin-top: 12px;
  background: transparent;
  border-top: none;
  width: 100%;
}
.footer__inner {
  min-height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 10px;
  padding: 8px 0;
}
.footer__sep {
  color: var(--border);
  font-size: 11px;
  line-height: 1;
  user-select: none;
}

.footer-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  text-decoration: none;
  color: inherit;
  font-size: 12px;
  line-height: 1.2;
  white-space: nowrap;
  opacity: 0.62;
  transition: opacity var(--transition);
}
.footer-item:hover {
  opacity: 1;
}
.footer-item__icon {
  color: var(--text-muted);
}
.footer-item__label {
  color: var(--text-muted);
}
.footer-item__label::after {
  content: ":";
}
.footer-item__value {
  color: var(--text-regular);
  font-weight: 500;
}

@media (max-width: 640px) {
  .footer__inner {
    gap: 8px;
  }
  .footer-item {
    font-size: 11px;
  }
}
</style>
