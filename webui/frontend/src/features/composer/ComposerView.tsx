import {
  type ComponentProps,
  type ComponentPropsWithRef,
  type ReactNode,
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
  /**
   * The goal/queued stack, rendered INSIDE the composer's own box (S72).
   *
   * It has to live here, not as a sibling above: Codex's stack derives its width
   * from the composer — inset 14px on each side — and the composer's own
   * horizontal padding changes across breakpoints. Anything computing that from
   * the outside guesses, and the guess was wrong the moment the layout narrowed
   * (measured 20px insets at 500px instead of 14). In here it inherits the same
   * padding and the same max-width, so the two edges stay parallel at every
   * width by construction.
   */
  goalStack?: ReactNode;
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
  goalStack,
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
      {/* G58② · loop/best only — goal renders as the Goal chip below, and
          the controller already withholds the prop for it. */}
      {launcher && <GoalLoopLauncher {...launcher} />}
      {goalStack}

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
          <div className="cx-bar-leading">
            <AddMenu {...addMenu} />
            <AccessPicker {...accessPicker} />
            {goalOptions && <GoalOptions {...goalOptions} />}
          </div>
          <div className="cx-bar-trailing">
            <ModelPicker {...modelPicker} />
            <AssistActions {...assistActions} />
            {deliveryModeControl && (
              <DeliveryModeControl {...deliveryModeControl} />
            )}
            <SubmitButton {...submitButton} />
          </div>
        </div>
      </div>

      <input type="file" multiple style={{ display: "none" }} {...fileInput} />
    </div>
  );
}
