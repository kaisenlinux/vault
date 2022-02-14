/**
 * @module GetCredentialsCard
 * GetCredentialsCard components are card-like components that display a title, and SearchSelect component that sends you to a credentials route for the selected item.
 * They are designed to be used in containers that act as flexbox or css grid containers.
 *
 * @example
 * ```js
 * <GetCredentialsCard @title="Get Credentials" @searchLabel="Role to use" @models={{array 'database/roles'}} @type="role" @backend={{model.backend}}/>
 * ```
 * @param title=null {String} - The title displays the card title
 * @param searchLabel=null {String} - The text above the searchSelect component
 * @param models=null {Array} - An array of model types to fetch from the API.  Passed through to SearchSelect component
 * @param type=null {String} - Determines where the transitionTo goes. If role to backend.credentials, if secret backend.show
 * @param shouldUseFallback=[false] {Boolean} - If true the input is used instead of search select.
 * @param subText=[null] {String} - Text below title
 * @param placeHolder=[null] {String} - Only works if shouldUseFallback is true. Displays the helper text inside the input.
 * @param backend=null {String} - Name of the backend to look up on search.
 */

import Component from '@glimmer/component';
import { inject as service } from '@ember/service';
import { action } from '@ember/object';
import { tracked } from '@glimmer/tracking';
export default class GetCredentialsCard extends Component {
  @service router;
  @service store;
  @tracked role = '';
  @tracked secret = '';

  @action
  transitionToCredential() {
    const role = this.role;
    const secret = this.secret;
    if (role) {
      this.router.transitionTo('vault.cluster.secrets.backend.credentials', role);
    }
    if (secret) {
      this.router.transitionTo('vault.cluster.secrets.backend.show', secret);
    }
  }

  get buttonDisabled() {
    return !this.role && !this.secret;
  }
  @action
  handleRoleInput(value) {
    if (this.args.type === 'role') {
      // if it comes in from the fallback component then the value is a string otherwise it's an array
      // which currently only happens if type is role.
      if (Array.isArray(value)) {
        this.role = value[0];
      } else {
        this.role = value;
      }
    }
    if (this.args.type === 'secret') {
      this.secret = value;
    }
  }
}
