import Controller from '@ember/controller';
import { inject as service } from '@ember/service';
import { tracked } from '@glimmer/tracking';

export default class OidcClientController extends Controller {
  @service router;
  @tracked isEditRoute;

  constructor() {
    super(...arguments);
    this.router.on(
      'routeDidChange',
      ({ targetName }) => (this.isEditRoute = targetName.includes('edit') ? true : false)
    );
  }

  get showHeader() {
    // hide header when rendering the edit form
    return !this.isEditRoute;
  }
}
