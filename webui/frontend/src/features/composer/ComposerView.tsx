import {
  type ComponentProps,
  type ComponentPropsWithRef,
} from "react";
import {
  AccessPicker,
  AddMenu,
  AssistActions,
  AttachmentList,
  BranchPicker,
  DeliveryModeControl,
  FileMentionMenu,
  GoalOptions,
  ModelPicker,
  ProjectPicker,
  RunLocationPicker,
  SlashCommandMenu,
  SubmitButton,
} from "./ComposerParts";
import { Textarea } from "../../ui/Field";
import {
  GoalLoopLauncher,
  type GoalLoopLauncherProps,
} from "./GoalLoopLauncher";

interface HomeEnvironmentViewProps {
  projectPicker: ComponentProps<typeof ProjectPicker>;
  runLocationPicker?: ComponentProps<typeof RunLocationPicker>;
  branchPicker?: ComponentProps<typeof BranchPicker>;
}

export interface ComposerViewProps {
  isSession: boolean;
  launcher?: GoalLoopLauncherProps;
  dragging: boolean;
  cardEvents: Pick<
    ComponentProps<"div">,
    "onDragEnter" | "onDragOver" | "onDragLeave" | "onDrop"
  >;
  environment?: HomeEnvironmentViewProps;
  attachments: ComponentProps<typeof AttachmentList>;
  textarea: ComponentPropsWithRef<typeof Textarea>;
  fileMentionMenu?: ComponentProps<typeof FileMentionMenu>;
  slashCommandMenu?: ComponentProps<typeof SlashCommandMenu>;
  addMenu: ComponentProps<typeof AddMenu>;
  accessPicker: ComponentProps<typeof AccessPicker>;
  goalOptions?: ComponentProps<typeof GoalOptions>;
  modelPicker: ComponentProps<typeof ModelPicker>;
  assistActions: ComponentProps<typeof AssistActions>;
  deliveryModeControl?: ComponentProps<typeof DeliveryModeControl>;
  submitButton: ComponentProps<typeof SubmitButton>;
  fileInput: ComponentPropsWithRef<"input">;
}

/**
 * Pure rendering boundary for the composer.
 *
 * The controller owns app services, store access, persistence and async
 * orchestration. This component only composes reusable UI from serializable
 * state, refs and callbacks supplied by its caller, which also makes it usable
 * in Storybook without an AppServices or store provider.
 */
export function ComposerView({
  isSession,
  launcher,
  dragging,
  cardEvents,
  environment,
  attachments,
  textarea,
  fileMentionMenu,
  slashCommandMenu,
  addMenu,
  accessPicker,
  goalOptions,
  modelPicker,
  assistActions,
  deliveryModeControl,
  submitButton,
  fileInput,
}: ComposerViewProps) {
  return (
    <div className={"cx " + (isSession ? "cx-session" : "cx-home")}>
      {launcher && launcher.mode !== "goal" && (
        <GoalLoopLauncher {...launcher} />
      )}

      <div
        className={"cx-card" + (dragging ? " dropping" : "")}
        {...cardEvents}
      >
        {environment && (
          <div className="cx-env-strip">
            <ProjectPicker {...environment.projectPicker} />
            {environment.runLocationPicker && environment.branchPicker && (
              <>
                <RunLocationPicker {...environment.runLocationPicker} />
                <BranchPicker {...environment.branchPicker} />
              </>
            )}
          </div>
        )}

        {dragging && (
          <div className="cx-drop absolute inset-0 z-[5] grid place-items-center rounded-[22px] border-2 border-dashed border-blue text-blue text-[13.5px] font-medium pointer-events-none">
            <span>Drop files to attach</span>
          </div>
        )}

        <AttachmentList {...attachments} />

        <div className="cx-input-wrap">
          <Textarea variant="unstyled" rows={1} {...textarea} />
        </div>

        {fileMentionMenu && <FileMentionMenu {...fileMentionMenu} />}
        {slashCommandMenu && <SlashCommandMenu {...slashCommandMenu} />}

        <div className="cx-bar">
          <AddMenu {...addMenu} />
          <AccessPicker {...accessPicker} />
          {goalOptions && <GoalOptions {...goalOptions} />}
          <span className="cx-spacer" />
          <ModelPicker {...modelPicker} />
          <AssistActions {...assistActions} />
          {deliveryModeControl && (
            <DeliveryModeControl {...deliveryModeControl} />
          )}
          <SubmitButton {...submitButton} />
        </div>
      </div>

      <input type="file" multiple style={{ display: "none" }} {...fileInput} />
    </div>
  );
}
