import Model, { attr } from '@ember-data/model';
export default class VersionHistoryModel extends Model {
  @attr('string') previousVersion;
  @attr('string') timestampInstalled;
}
