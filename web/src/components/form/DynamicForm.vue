<script setup lang="ts">
import { computed, ref } from "vue";
import type { FieldSchema } from "@/api/types";
import { buildFormLayoutRows } from "@/utils/formLayout";
import AppSelect from "@/components/base/AppSelect.vue";
import AppInput from "@/components/base/AppInput.vue";
import FormField from "@/components/base/FormField.vue";
import AccountFolderField from "@/components/admin/AccountFolderField.vue";
import LocalDirBrowserModal from "@/components/common/LocalDirBrowserModal.vue";

const props = defineProps<{ fields: FieldSchema[]; modelValue: Record<string, unknown> }>();
const emit = defineEmits<{ "update:modelValue": [Record<string, unknown>] }>();

const layoutRows = computed(() => buildFormLayoutRows(props.fields));
const browseOpen = ref(false);
const browseField = ref("");

function setField(name: string, value: unknown) {
  const next = { ...props.modelValue, [name]: value };
  const previousKey = String(props.modelValue[name] ?? "");
  const nextKey = String(value ?? "");
  for (const field of props.fields) {
    if (field.default_by !== name || !field.defaults || !(nextKey in field.defaults)) continue;
    const current = String(props.modelValue[field.name] ?? "").trim();
    const previousDefault = field.defaults[previousKey];
    if (!current || current === previousDefault || current === field.default) {
      next[field.name] = field.defaults[nextKey];
    }
  }
  emit("update:modelValue", next);
}

function selectOptions(f: FieldSchema) {
  return (f.options ?? []).map((o) => {
    if (o.value === "true") return { value: true, label: o.label };
    if (o.value === "false") return { value: false, label: o.label };
    return { value: o.value, label: o.label };
  });
}

function openBrowse(fieldName: string) {
  browseField.value = fieldName;
  browseOpen.value = true;
}

function onBrowseSelect(path: string) {
  if (browseField.value) setField(browseField.value, path);
  browseOpen.value = false;
  browseField.value = "";
}

function fieldString(name: string): string {
  const v = props.modelValue[name];
  return typeof v === "string" ? v : v == null ? "" : String(v);
}
</script>

<template>
  <div class="dyn-form">
    <div
      v-for="(row, rowIdx) in layoutRows"
      :key="'row-' + rowIdx"
      :class="row.mode === 'full' ? 'dyn-form__row-full' : 'dyn-form__row-half'"
    >
      <FormField
        v-for="f in row.fields"
        :key="f.name"
        :label="f.label"
        :required="f.required"
      >
        <AppSelect
          v-if="f.type === 'select'"
          :model-value="(modelValue[f.name] as string) ?? ''"
          :options="selectOptions(f)"
          @update:model-value="(v) => setField(f.name, v)"
        />

        <label v-else-if="f.type === 'bool'" class="switch">
          <input
            type="checkbox"
            :checked="Boolean(modelValue[f.name])"
            @change="(e) => setField(f.name, (e.target as HTMLInputElement).checked)"
          />
          <span class="switch__slider" />
        </label>

        <AccountFolderField
          v-else-if="f.type === 'local_dir'"
          :display="fieldString(f.name)"
          :title="fieldString(f.name)"
          placeholder="点击浏览选择容器内目录"
          browse-label="浏览"
          @browse="openBrowse(f.name)"
        />

        <textarea
          v-else-if="f.type === 'textarea'"
          class="dyn-form__textarea"
          rows="5"
          :value="(modelValue[f.name] as string) ?? ''"
          :placeholder="f.default ? `默认：${f.default}` : ''"
          @input="(e) => setField(f.name, (e.target as HTMLTextAreaElement).value)"
        />

        <AppInput
          v-else
          :type="f.type === 'password' ? 'password' : f.type === 'number' ? 'number' : 'text'"
          :model-value="(modelValue[f.name] as string) ?? ''"
          :placeholder="f.default ? `默认：${f.default}` : ''"
          @update:model-value="(v) => setField(f.name, v)"
        />
      </FormField>
    </div>

    <LocalDirBrowserModal
      :open="browseOpen"
      :initial-path="browseField ? fieldString(browseField) : ''"
      @close="browseOpen = false"
      @select="onBrowseSelect"
    />
  </div>
</template>

<style scoped>
.dyn-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dyn-form__row-full {
  display: block;
}

.dyn-form__row-half {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 20px;
  align-items: start;
}

@media (max-width: 640px) {
  .dyn-form__row-half {
    grid-template-columns: 1fr;
  }
}

.dyn-form__textarea {
  width: 100%;
  min-height: 96px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  line-height: 1.55;
  resize: vertical;
  box-sizing: border-box;
}

.dyn-form__textarea:focus {
  outline: none;
  border-color: var(--brand);
}

.switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
}
.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}
.switch__slider {
  position: absolute;
  inset: 0;
  background: var(--border);
  border-radius: var(--radius-pill);
  transition: var(--transition);
}
.switch__slider::before {
  content: "";
  position: absolute;
  height: 18px;
  width: 18px;
  left: 3px;
  top: 3px;
  background: #fff;
  border-radius: 50%;
  transition: var(--transition);
}
.switch input:checked + .switch__slider {
  background: var(--brand);
}
.switch input:checked + .switch__slider::before {
  transform: translateX(20px);
}
</style>
