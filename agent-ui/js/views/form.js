import LoginBtn from "../components/login-controls.js";
import LoginForm from "../components/login-form.js";
import Observable from "./observable.js";
import DownloadControls from "../components/download-controls.js";
import FormStates from "../commons/states.js";
import InfoDiscovery from "../components/info.js";

export default class FormView extends Observable {
    constructor(store, formElement, btnElement) {
        super({
            store: store,
        })

        let self = this;
        self.formElement = formElement;
        self.btnElement = btnElement;
    }

    clean() {
        let self = this;
        self.formElement.innerHTML = "";
        self.btnElement.innerHTML = "";
    }

    render() {
        let self = this;

        self.clean();

        // if the inventory is gathered don't show the login form.
        if (self.store.state.formState == FormStates.UpToDate) {
            let info = new InfoDiscovery().render();
            let btns = new DownloadControls({
                downloadCallback: () => {
                    window.open(
                        window.location.origin + "/api/v1/inventory",
                        "_blank"
                    )
                },
                goBackBtnCallback: () => {
                    const serviceUrl = self.store.state.url || "http://localhost:3000/migrate/wizard";
                    window.open(serviceUrl, '_blank', 'noopener,noreferrer');
                },
            }).render();

            self.formElement.appendChild(info);
            self.btnElement.appendChild(btns);
            return;
        }

        // render login form
        const form = new LoginForm({
            dataSharingEnabled: self.store.state.dataSharingAccepted,
            disabled: self.store.state.requestPending || self.store.state.formState == FormStates.CredentialsAccepted || self.store.state.formState == FormStates.GatheringInventory,
            enableDataSharingCallback: (elem) => {
                self.store.dispatch("enableDataSharing", {
                    enabled: elem.checked
                });
            },
            credentials: self.store.state.creds,
            state: self.store.state.formState,
            valid: self.store.state.formValidation.valid,
            invalidElement: self.store.state.formValidation.element,
            urlHelperText: "Please enter an URL",
            usernameHelperText: "Please enter an email address",
        }).render();

        let msg = "Log in...";
        switch (self.store.state.formState) {
            case FormStates.CredentialsAccepted || FormStates.GatheringInventory:
                msg = "Gathering inventory..."
                break;
            default:
                "Log in...";
        }

        let btns = null;
        btns = new LoginBtn({
            msg: msg,
            isButtonEnabled: () => {
                if (self.store.state.requestPending) {
                    return false;
                }
                if (!self.store.state.dataSharingAccepted) {
                    return false;
                }

                return self.store.state.formState != FormStates.CredentialsAccepted && self.store.state.formState != FormStates.GatheringInventory
            },
            spinnerEnabled: self.store.state.requestPending || self.store.state.formState == FormStates.CredentialsAccepted || self.store.state.formState == FormStates.GatheringInventory,
            postCallback: () => {
                const credentials = {
                    url: form.elements['url'].value,
                    username: form.elements['username'].value,
                    password: form.elements['password'].value,
                }
                self.store.dispatch('formCredentials', credentials);


                const reUrl = /^(https?:\/\/)?([\w-]+\.)+[\w-]+(\/[\w-./?%&=]*)?$/
                if (!reUrl.test(credentials.url)) {
                    self.store.dispatch('formValidation', { valid: false, element: "url" });
                    return
                }

                // validate the form first
                const reEmail = /^(([^<>()\[\]\\.,;:\s@"]+(\.[^<>()\[\]\\.,;:\s@"]+)*)|(".+"))@((\[[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\])|(([a-zA-Z\-0-9]+\.)+[a-zA-Z]{2,}))$/i;
                if (!reEmail.test(credentials.username)) {
                    self.store.dispatch('formValidation', { valid: false, element: "username" });
                    return
                }

                // post them
                self.store.dispatch('postCredentials', {
                    ...credentials,
                    isDataSharingAllowed: self.store.state.dataSharingAccepted,
                });
            },
        }).render();

        self.formElement.appendChild(form);
        self.btnElement.appendChild(btns);
    }
}
