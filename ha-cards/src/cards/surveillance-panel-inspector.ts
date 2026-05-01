import { html, nothing, type TemplateResult } from "lit";

import type { PanelModel } from "../domain/model";
import { renderCameraInspector } from "./surveillance-panel-inspector-camera";
import { renderNvrInspector } from "./surveillance-panel-inspector-nvr";
import {
  type IsBusyFn,
  type OnVtoButtonAction,
  type OnVtoRangeChange,
  type OnVtoSwitchAction,
  type RenderIconFn,
} from "./surveillance-panel-inspector-shared";
import { renderVtoInspector } from "./surveillance-panel-inspector-vto";
import type { DetailTab } from "./surveillance-panel-state";

interface RenderSurveillancePanelInspectorArgs {
  model: PanelModel;
  inspectorOpen: boolean;
  errorMessage: string;
  detailTab: DetailTab;
  eventContent: TemplateResult | typeof nothing;
  archiveContent: TemplateResult | typeof nothing;
  mp4Content: TemplateResult | typeof nothing;
  renderIcon: RenderIconFn;
  isBusy: IsBusyFn;
  onSelectDetailTab: (tab: DetailTab) => void;
  onVtoRangeChange: OnVtoRangeChange;
  onVtoSwitchAction: OnVtoSwitchAction;
  onVtoButtonAction: OnVtoButtonAction;
}

export function renderSurveillancePanelInspector({
  model,
  inspectorOpen,
  errorMessage,
  detailTab,
  eventContent,
  archiveContent,
  mp4Content,
  renderIcon,
  isBusy,
  onSelectDetailTab,
  onVtoRangeChange,
  onVtoSwitchAction,
  onVtoButtonAction,
}: RenderSurveillancePanelInspectorArgs): TemplateResult | typeof nothing {
  if (!model.selectedCamera && !model.selectedNvr && !model.selectedVto) {
    return nothing;
  }

  return html`
    <aside class="inspector ${inspectorOpen ? "" : "mobile-hidden"}">
      ${errorMessage ? html`<div class="error-banner">${errorMessage}</div>` : nothing}
      ${model.selectedCamera
        ? renderCameraInspector(
            model.selectedCamera,
            detailTab,
            eventContent,
            archiveContent,
            mp4Content,
            onSelectDetailTab,
          )
        : model.selectedNvr
          ? renderNvrInspector(
              model.selectedNvr,
              renderIcon,
              detailTab,
              archiveContent,
              onSelectDetailTab,
            )
          : model.selectedVto
            ? renderVtoInspector(
                model.selectedVto,
                detailTab,
                eventContent,
                renderIcon,
                isBusy,
                onSelectDetailTab,
                onVtoRangeChange,
                onVtoSwitchAction,
                onVtoButtonAction,
              )
            : nothing}
    </aside>
  `;
}
